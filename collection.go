package oltgo

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type ctxKey struct{}

var collectionKey = ctxKey{}

// WithCollection injeta uma instância de *Collection no contexto fornecido.
func WithCollection(ctx context.Context, c *Collection) context.Context {
	return context.WithValue(ctx, collectionKey, c)
}

// FromContext recupera a instância de *Collection do contexto.
// Retorna nil se não estiver presente.
func FromContext(ctx context.Context) *Collection {
	if c, ok := ctx.Value(collectionKey).(*Collection); ok {
		return c
	}
	return nil
}

type Collection struct {
	mu            sync.Mutex
	agent         *Agent
	tracingID     string
	environment   Environment
	schemaVersion string
	service       Service
	request       *Request
	actor         *Actor
	createdAt     time.Time
	events        []Event
	eventCounter  int64
	committed     bool
}

// NewCollection inicializa uma nova Collection vinculada ao Agent.
func NewCollection(agent *Agent, tracingID string) *Collection {
	return &Collection{
		agent:         agent,
		tracingID:     tracingID,
		environment:   agent.Env,
		schemaVersion: agent.SchemaVersion,
		service:       agent.Service,
		createdAt:     time.Now(),
		events:        make([]Event, 0),
	}
}

// SetRequest define informações de requisição de forma thread-safe.
// É opcional: traces sem requisição (inicialização, jobs, workers, CLIs) podem
// nunca chamar este método — o campo "request" será omitido do JSON final.
// Passar nil também limpa uma requisição previamente definida.
func (c *Collection) SetRequest(req *Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.request = req
}

// SetRequestFromHTTP preenche informações da requisição a partir de um *http.Request nativo.
// Se r for nil, a chamada é ignorada e o campo "request" permanece omitido.
func (c *Collection) SetRequestFromHTTP(r *http.Request, comm CommunicationProto, status int) {
	if r == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.request = &Request{
		Communication: comm,
		Method:        r.Method,
		Route:         r.URL.Path,
		Status:        status,
		UserAgent:     r.UserAgent(),
	}
}

// SetActor define informações do ator da requisição.
func (c *Collection) SetActor(actor *Actor) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.actor = actor
}

// AddEvent insere um evento completo de forma thread-safe.
//
// O status é derivado da severidade quando não informado: eventos com severidade
// ERROR ou FATAL viram FAILURE. Um Status explícito é sempre respeitado.
func (c *Collection) AddEvent(event Event) {
	// Seguro aqui porque a duração destes eventos já vem preenchida pelo chamador.
	event.Timestamp = event.Timestamp.UTC()

	// Um evento com erro registrado é sempre falha, e nunca abaixo de ERROR.
	if len(event.Errors) > 0 {
		event.Status = StatusFailure
		if !IsFailureSeverity(event.Severity) {
			event.Severity = SeverityError
		}
	}

	// Status não informado: deriva da severidade. O zero value "" distingue
	// "não informado" de "informado como SUCCESS", então um status explícito é
	// respeitado — inclusive SUCCESS num evento de severidade ERROR/FATAL, que
	// é o caso do erro tratado com a execução seguindo por outro caminho.
	if event.Status == "" {
		if IsFailureSeverity(event.Severity) {
			event.Status = StatusFailure
		} else {
			event.Status = StatusSuccess
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

// Commit finaliza a trace, calcula a duração e submete os dados para o Shipping do Agent.
// É idempotente: uma Collection representa um trace de uso único, e chamadas
// repetidas são no-op.
func (c *Collection) Commit() {
	c.mu.Lock()
	if c.committed {
		c.mu.Unlock()
		return
	}
	c.committed = true

	finishedAt := time.Now()
	// Duração calculada ANTES de qualquer .UTC(): preserva o relógio monotônico.
	durationMs := int(finishedAt.Sub(c.createdAt).Milliseconds())
	if durationMs < 0 {
		durationMs = 0
	}

	overallStatus := StatusSuccess
	for _, evt := range c.events {
		if evt.Status == StatusFailure {
			overallStatus = StatusFailure
			break
		}
	}

	if c.request != nil && c.request.Status >= 400 {
		overallStatus = StatusFailure
	}

	log := LogSchema{
		TracingID:     c.tracingID,
		Status:        overallStatus,
		Environment:   c.environment,
		SchemaVersion: c.schemaVersion,
		Service:       c.service,
		Request:       c.request,
		Actor:         c.actor,
		Time: TimeInfo{
			DurationMs: durationMs,
			// .UTC() apenas aqui: os valores já não participam de cálculo.
			CreatedAt:  c.createdAt.UTC(),
			FinishedAt: finishedAt.UTC(),
		},
		Events: buildEventTree(c.events),
	}
	c.mu.Unlock()

	c.agent.Shipping.Ship(log)
}

// buildEventTree monta a árvore de eventos a partir da lista plana, garantindo
// que todo evento de entrada apareça exatamente uma vez na saída.
func buildEventTree(flatEvents []Event) []*Event {
	nodes := make([]*Event, len(flatEvents))
	byID := make(map[string]*Event, len(flatEvents))

	for i := range flatEvents {
		evtCopy := flatEvents[i]
		evtCopy.Events = make([]*Event, 0)
		nodes[i] = &evtCopy
		// Em caso de event_id duplicado, a primeira ocorrência mantém o id.
		// As demais permanecem na árvore como nós independentes.
		if _, dup := byID[evtCopy.EventID]; !dup {
			byID[evtCopy.EventID] = nodes[i]
		}
	}

	// Cada evento tem 0 ou 1 pai: o grafo de ancestrais é funcional.
	parentOf := make(map[*Event]*Event, len(nodes))
	for _, n := range nodes {
		if n.ParentEventID == nil || *n.ParentEventID == "" {
			continue
		}
		if parent, ok := byID[*n.ParentEventID]; ok {
			parentOf[n] = parent
		}
		// Pai inexistente: sem entrada em parentOf → promovido a raiz,
		// preservando o comportamento anterior para eventos órfãos.
	}

	cyclic := detectCyclic(nodes, parentOf)

	var rootEvents []*Event
	for _, node := range nodes {
		parent, hasParent := parentOf[node]
		if !hasParent || cyclic[node] {
			rootEvents = append(rootEvents, node)
			continue
		}
		parent.Events = append(parent.Events, node)
	}

	return rootEvents
}

// detectCyclic marca os eventos que participam de um ciclo de ancestrais.
// Nós em ciclo são promovidos a raiz em vez de descartados, e a ligação com o
// pai é omitida — o que também evita a recursão infinita na serialização JSON.
//
// Custo O(n): cada nó entra em estado "walking" no máximo uma vez em todo o
// processamento, independentemente de quantos nós iniciem uma caminhada.
func detectCyclic(nodes []*Event, parentOf map[*Event]*Event) map[*Event]bool {
	const (
		pending = 0
		walking = 1
		settled = 2
	)
	state := make(map[*Event]int, len(nodes))
	cyclic := make(map[*Event]bool)

	for _, start := range nodes {
		if state[start] != pending {
			continue
		}
		var path []*Event
		for cur := start; cur != nil; cur = parentOf[cur] {
			if state[cur] == walking {
				// Reencontrou um nó do caminho atual: marca o ciclo.
				for i := len(path) - 1; i >= 0; i-- {
					cyclic[path[i]] = true
					if path[i] == cur {
						break
					}
				}
				break
			}
			if state[cur] == settled {
				break
			}
			state[cur] = walking
			path = append(path, cur)
		}
		for _, n := range path {
			state[n] = settled
		}
	}

	return cyclic
}

type activeEventKey struct{}

// WithActiveEvent associa um ID de evento ao contexto.
func WithActiveEvent(ctx context.Context, eventID string) context.Context {
	return context.WithValue(ctx, activeEventKey{}, eventID)
}

// ActiveEventFromContext recupera o ID do evento pai do contexto.
func ActiveEventFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(activeEventKey{}).(string); ok {
		return id
	}
	return ""
}

// StartEvent inicia a gravação de um evento usando context.Context. O evento herdará o ID do pai automaticamente.
func StartEvent(ctx context.Context, name string, eventType EventType, severity Severity) (context.Context, *EventBuilder) {
	c := FromContext(ctx)
	if c == nil {
		return ctx, &EventBuilder{}
	}

	idNum := atomic.AddInt64(&c.eventCounter, 1)
	eventID := fmt.Sprintf("evt_%06d", idNum)

	eb := &EventBuilder{
		collection: c,
		event: Event{
			EventID:   eventID,
			Name:      name,
			Type:      eventType,
			Severity:  severity,
			Timestamp: time.Now(),
			Status:    StatusSuccess,
		},
	}

	if parentID := ActiveEventFromContext(ctx); parentID != "" {
		eb.event.ParentEventID = &parentID
	}

	newCtx := WithActiveEvent(ctx, eventID)
	return newCtx, eb
}

// EventBuilder auxilia desenvolvedores a construírem eventos com cálculo de tempo automático.
type EventBuilder struct {
	collection *Collection
	event      Event
	statusSet  bool // status definido explicitamente por Fail() ou Succeed()
}

// StartEvent inicia a gravação de um evento. O evento será registrado no Collection chamando End().
func (c *Collection) StartEvent(name string, eventType EventType, severity Severity) *EventBuilder {
	idNum := atomic.AddInt64(&c.eventCounter, 1)
	eventID := fmt.Sprintf("evt_%06d", idNum)

	return &EventBuilder{
		collection: c,
		event: Event{
			EventID:   eventID,
			Name:      name,
			Type:      eventType,
			Severity:  severity,
			Timestamp: time.Now(),
			Status:    StatusSuccess,
		},
	}
}

// WithParent define o ID do evento pai para construir árvores/spans.
func (eb *EventBuilder) WithParent(parentID string) *EventBuilder {
	if eb == nil {
		return nil
	}
	eb.event.ParentEventID = &parentID
	return eb
}

// WithMessage adiciona uma mensagem explicativa ao evento.
func (eb *EventBuilder) WithMessage(msg string) *EventBuilder {
	if eb == nil {
		return nil
	}
	eb.event.Message = msg
	return eb
}

// WithMetadata injeta dados de contexto adicionais de forma flexível.
func (eb *EventBuilder) WithMetadata(metadata map[string]any) *EventBuilder {
	if eb == nil {
		return nil
	}
	eb.event.Metadata = metadata
	return eb
}

// Fail marca o evento como falho sem exigir um objeto de erro. Use quando a
// operação não teve sucesso mas não há código/mensagem de erro a registrar.
func (eb *EventBuilder) Fail() *EventBuilder {
	if eb == nil {
		return nil
	}
	eb.event.Status = StatusFailure
	eb.statusSet = true
	return eb
}

// Succeed marca o evento como bem-sucedido explicitamente. Necessário apenas
// para manter SUCCESS em um evento com severidade ERROR ou FATAL, que de outro
// modo seria derivado como FAILURE em End().
func (eb *EventBuilder) Succeed() *EventBuilder {
	if eb == nil {
		return nil
	}
	eb.event.Status = StatusSuccess
	eb.statusSet = true
	return eb
}

// AddError adiciona um erro ocorrido durante a operação do evento, definindo o
// status do evento como FAILURE e elevando a severidade para no mínimo ERROR.
func (eb *EventBuilder) AddError(code, message string, stack *string) *EventBuilder {
	if eb == nil {
		return nil
	}
	eb.event.Status = StatusFailure
	eb.statusSet = true
	// Um evento com erro registrado não pode permanecer abaixo de ERROR.
	if !IsFailureSeverity(eb.event.Severity) {
		eb.event.Severity = SeverityError
	}
	eb.event.Errors = append(eb.event.Errors, EventError{
		Code:    code,
		Message: message,
		Stack:   stack,
	})
	return eb
}

// End calcula o tempo de duração final do evento e o adiciona à Collection.
func (eb *EventBuilder) End() {
	if eb == nil || eb.collection == nil {
		return
	}
	// Duração primeiro, com o relógio monotônico intacto...
	eb.event.DurationMs = int(time.Since(eb.event.Timestamp).Milliseconds())
	if eb.event.DurationMs < 0 {
		eb.event.DurationMs = 0
	}
	// ...e só então normaliza o timestamp para UTC.
	eb.event.Timestamp = eb.event.Timestamp.UTC()
	// Severidade ERROR/FATAL implica falha, a menos que o status tenha sido
	// definido explicitamente via Fail() ou Succeed().
	if !eb.statusSet && IsFailureSeverity(eb.event.Severity) {
		eb.event.Status = StatusFailure
	}
	eb.collection.AddEvent(eb.event)
}
