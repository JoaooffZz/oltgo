package oltgo

import (
	"context"
	"fmt"
	"net/http"
	"sync"
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
func (c *Collection) SetRequest(req *Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.request = req
}

// SetRequestFromHTTP preenche informações da requisição a partir de um *http.Request nativo.
func (c *Collection) SetRequestFromHTTP(r *http.Request, comm CommunicationProto, status int) {
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
func (c *Collection) AddEvent(event Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

// Commit finaliza a trace, calcula a duração e submete os dados para o Shipping do Agent.
func (c *Collection) Commit() {
	c.mu.Lock()
	finishedAt := time.Now()
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
			CreatedAt:  c.createdAt,
			FinishedAt: finishedAt,
		},
		Events: c.events,
	}
	c.mu.Unlock()

	c.agent.Shipping.Ship(log)
}

// EventBuilder auxilia desenvolvedores a construírem eventos com cálculo de tempo automático.
type EventBuilder struct {
	collection *Collection
	event      Event
}

// StartEvent inicia a gravação de um evento. O evento será registrado no Collection chamando End().
func (c *Collection) StartEvent(name string, eventType EventType, severity Severity) *EventBuilder {
	c.mu.Lock()
	idNum := len(c.events) + 1
	c.mu.Unlock()

	eventID := fmt.Sprintf("evt_%03d", idNum)

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
	eb.event.ParentEventID = &parentID
	return eb
}

// WithMessage adiciona uma mensagem explicativa ao evento.
func (eb *EventBuilder) WithMessage(msg string) *EventBuilder {
	eb.event.Message = msg
	return eb
}

// WithMetadata injeta dados de contexto adicionais de forma flexível.
func (eb *EventBuilder) WithMetadata(metadata map[string]any) *EventBuilder {
	eb.event.Metadata = metadata
	return eb
}

// AddError adiciona um erro ocorrido durante a operação do evento, definindo o status do evento como FAILURE.
func (eb *EventBuilder) AddError(code, message string, stack *string) *EventBuilder {
	eb.event.Status = StatusFailure
	eb.event.Errors = append(eb.event.Errors, EventError{
		Code:    code,
		Message: message,
		Stack:   stack,
	})
	return eb
}

// End calcula o tempo de duração final do evento e o adiciona à Collection.
func (eb *EventBuilder) End() {
	eb.event.DurationMs = int(time.Since(eb.event.Timestamp).Milliseconds())
	if eb.event.DurationMs < 0 {
		eb.event.DurationMs = 0
	}
	eb.collection.AddEvent(eb.event)
}
