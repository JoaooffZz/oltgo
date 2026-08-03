package tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joaooffzz/oltgo"
)

func TestContextPropagation(t *testing.T) {
	service := oltgo.Service{
		Name:    "test-service",
		Version: "1.0.0",
	}
	agent := oltgo.NewAgent(service, oltgo.Testing, "1", nil)
	defer agent.Close()

	col := agent.NewCollection("trace-123")

	ctx := oltgo.WithCollection(context.Background(), col)
	retrieved := oltgo.FromContext(ctx)

	if retrieved != col {
		t.Errorf("expected to retrieve the same collection from context")
	}

	nilRetrieved := oltgo.FromContext(context.Background())
	if nilRetrieved != nil {
		t.Errorf("expected to retrieve nil from empty context")
	}
}

func TestCollectionAndShippingWithCallback(t *testing.T) {
	service := oltgo.Service{
		Name:    "payment-service",
		Version: "1.2.3",
	}

	var processedLog oltgo.LogSchema
	var mu sync.Mutex
	called := false

	processLog := func(log oltgo.LogSchema) error {
		mu.Lock()
		defer mu.Unlock()
		processedLog = log
		called = true
		return nil
	}

	agent := oltgo.NewAgent(service, oltgo.Testing, "1", processLog)

	col := agent.NewCollection("trace-abc")

	req, _ := http.NewRequest("POST", "/checkout", nil)
	req.Header.Set("User-Agent", "Go-Test")
	col.SetRequestFromHTTP(req, oltgo.ProtoREST, 200)

	col.SetActor(&oltgo.Actor{
		ID:   "usr_999",
		Type: "USER",
		IP:   "127.0.0.1",
	})

	// Add a successful event
	event1 := col.StartEvent("auth", oltgo.TypeFunction, oltgo.SeverityInfo).
		WithMessage("auth success").
		WithMetadata(map[string]any{"method": "API_KEY"})
	time.Sleep(2 * time.Millisecond)
	event1.End()

	// Add a failed event
	event2 := col.StartEvent("stripe_charge", oltgo.TypeExternalService, oltgo.SeverityError).
		WithMessage("charge failed").
		AddError("CARD_DECLINED", "declined", nil)
	event2.End()

	col.Commit()

	// Wait for shipping to process the log (since it is asynchronous)
	agent.Close() // Close will flush and wait for workers to complete

	mu.Lock()
	defer mu.Unlock()

	if !called {
		t.Fatal("expected ProcessLog callback to be called")
	}

	if processedLog.TracingID != "trace-abc" {
		t.Errorf("expected TracingID trace-abc, got %s", processedLog.TracingID)
	}

	if processedLog.Status != oltgo.StatusFailure {
		t.Errorf("expected overall Status to be FAILURE, got %s", processedLog.Status)
	}

	if len(processedLog.Events) != 2 {
		t.Errorf("expected 2 root events, got %d", len(processedLog.Events))
	}

	if processedLog.Events[0].Name != "auth" || processedLog.Events[0].Status != oltgo.StatusSuccess {
		t.Errorf("first event check failed")
	}

	if processedLog.Events[1].Name != "stripe_charge" || processedLog.Events[1].Status != oltgo.StatusFailure {
		t.Errorf("second event check failed")
	}
}

func TestCollectionAndShippingDiscard(t *testing.T) {
	service := oltgo.Service{
		Name:    "discard-service",
		Version: "1.0.0",
	}

	// No processLog function (nil callback)
	agent := oltgo.NewAgent(service, oltgo.Testing, "1", nil)

	col := agent.NewCollection("trace-discard")
	col.Commit()

	// Closing the agent to ensure worker finishes processing
	agent.Close()
}

func TestInitializationLogOmitsRequest(t *testing.T) {
	service := oltgo.Service{
		Name:    "bootstrap-service",
		Version: "1.0.0",
	}

	var processedLog oltgo.LogSchema
	var mu sync.Mutex
	called := false

	processLog := func(log oltgo.LogSchema) error {
		mu.Lock()
		defer mu.Unlock()
		processedLog = log
		called = true
		return nil
	}

	agent := oltgo.NewAgent(service, oltgo.Testing, "1", processLog)

	// Trace de inicialização: nenhum SetRequest / SetActor é chamado.
	col := agent.NewCollection("trace-startup")
	ctx := oltgo.WithCollection(context.Background(), col)

	ctx, bootEvt := oltgo.StartEvent(ctx, "bootstrap", oltgo.TypeFunction, oltgo.SeverityInfo)
	bootEvt.WithMessage("inicializando serviço")

	_, dbEvt := oltgo.StartEvent(ctx, "connect_database", oltgo.TypeDatabase, oltgo.SeverityInfo)
	dbEvt.WithMessage("pool de conexões estabelecido").End()

	bootEvt.End()

	col.Commit()
	agent.Close()

	mu.Lock()
	defer mu.Unlock()

	if !called {
		t.Fatal("expected ProcessLog callback to be called")
	}

	if processedLog.Request != nil {
		t.Errorf("expected Request to be nil for an initialization log, got %+v", processedLog.Request)
	}

	if processedLog.Actor != nil {
		t.Errorf("expected Actor to be nil for an initialization log, got %+v", processedLog.Actor)
	}

	if processedLog.Status != oltgo.StatusSuccess {
		t.Errorf("expected overall Status SUCCESS without a request, got %s", processedLog.Status)
	}

	if len(processedLog.Events) != 1 {
		t.Fatalf("expected 1 root event, got %d", len(processedLog.Events))
	}

	data, err := json.Marshal(processedLog)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	payload := string(data)
	if strings.Contains(payload, `"request"`) {
		t.Errorf("expected the \"request\" key to be omitted from the JSON, got %s", payload)
	}
	if strings.Contains(payload, `"actor"`) {
		t.Errorf("expected the \"actor\" key to be omitted from the JSON, got %s", payload)
	}
	if !strings.Contains(payload, `"tracing_id":"trace-startup"`) {
		t.Errorf("expected tracing_id in the JSON, got %s", payload)
	}
}

func TestSetRequestNilClearsRequest(t *testing.T) {
	service := oltgo.Service{
		Name:    "reset-service",
		Version: "1.0.0",
	}

	var processedLog oltgo.LogSchema
	var mu sync.Mutex

	processLog := func(log oltgo.LogSchema) error {
		mu.Lock()
		defer mu.Unlock()
		processedLog = log
		return nil
	}

	agent := oltgo.NewAgent(service, oltgo.Testing, "1", processLog)

	col := agent.NewCollection("trace-reset")
	col.SetRequest(&oltgo.Request{
		Communication: oltgo.ProtoREST,
		Method:        "GET",
		Route:         "/health",
		Status:        200,
	})
	col.SetRequest(nil)

	// SetRequestFromHTTP com r nil deve ser seguro e não definir requisição.
	col.SetRequestFromHTTP(nil, oltgo.ProtoREST, 200)

	col.Commit()
	agent.Close()

	mu.Lock()
	defer mu.Unlock()

	if processedLog.Request != nil {
		t.Errorf("expected Request to be nil after SetRequest(nil), got %+v", processedLog.Request)
	}
}

// countAllEvents conta recursivamente todos os eventos na árvore.
func countAllEvents(events []*oltgo.Event) int {
	count := len(events)
	for _, evt := range events {
		count += countAllEvents(evt.Events)
	}
	return count
}

func TestCollectionConcurrencySafety(t *testing.T) {
	service := oltgo.Service{
		Name:    "concurrent-service",
		Version: "1.0.0",
	}

	var processedCount int
	var mu sync.Mutex

	processLog := func(log oltgo.LogSchema) error {
		mu.Lock()
		defer mu.Unlock()
		processedCount += countAllEvents(log.Events)
		return nil
	}

	agent := oltgo.NewAgent(service, oltgo.Testing, "1", processLog)

	col := agent.NewCollection("trace-concurrent")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			event := col.StartEvent(errors.New("").Error(), oltgo.TypeFunction, oltgo.SeverityDebug)
			event.End()
		}(i)
	}

	wg.Wait()
	col.Commit()

	agent.Close()

	mu.Lock()
	defer mu.Unlock()

	if processedCount != 50 {
		t.Errorf("expected 50 events processed, got %d", processedCount)
	}
}

func TestHierarchicalEventsWithContext(t *testing.T) {
	service := oltgo.Service{
		Name:    "hierarchy-service",
		Version: "1.0.0",
	}

	var processedLog oltgo.LogSchema
	var mu sync.Mutex
	called := false

	processLog := func(log oltgo.LogSchema) error {
		mu.Lock()
		defer mu.Unlock()
		processedLog = log
		called = true
		return nil
	}

	agent := oltgo.NewAgent(service, oltgo.Testing, "1", processLog)

	col := agent.NewCollection("trace-hierarchy")
	ctx := oltgo.WithCollection(context.Background(), col)

	// Evento raiz: process_order
	ctx, mainEvt := oltgo.StartEvent(ctx, "process_order", oltgo.TypeFunction, oltgo.SeverityInfo)
	mainEvt.WithMessage("processing order")

	// Subevento 1: validate_cart (filho de process_order)
	ctx2, validateEvt := oltgo.StartEvent(ctx, "validate_cart", oltgo.TypeFunction, oltgo.SeverityInfo)
	validateEvt.WithMessage("cart validated")
	validateEvt.End()

	// Subevento 1.1: check_stock (filho de validate_cart)
	_, stockEvt := oltgo.StartEvent(ctx2, "check_stock", oltgo.TypeDatabase, oltgo.SeverityInfo)
	stockEvt.WithMessage("stock checked")
	stockEvt.End()

	// Subevento 2: authorize_payment (filho de process_order)
	_, payEvt := oltgo.StartEvent(ctx, "authorize_payment", oltgo.TypeExternalService, oltgo.SeverityInfo)
	payEvt.WithMessage("payment authorized")
	payEvt.End()

	mainEvt.End()

	col.Commit()
	agent.Close()

	mu.Lock()
	defer mu.Unlock()

	if !called {
		t.Fatal("expected ProcessLog callback to be called")
	}

	// Deve haver 1 evento raiz: "process_order"
	if len(processedLog.Events) != 1 {
		t.Fatalf("expected 1 root event, got %d", len(processedLog.Events))
	}

	root := processedLog.Events[0]
	if root.Name != "process_order" {
		t.Errorf("expected root event name 'process_order', got '%s'", root.Name)
	}

	// O root deve ter 2 filhos diretos: validate_cart e authorize_payment
	if len(root.Events) != 2 {
		t.Fatalf("expected 2 child events under root, got %d", len(root.Events))
	}

	validateChild := root.Events[0]
	if validateChild.Name != "validate_cart" {
		t.Errorf("expected first child name 'validate_cart', got '%s'", validateChild.Name)
	}

	payChild := root.Events[1]
	if payChild.Name != "authorize_payment" {
		t.Errorf("expected second child name 'authorize_payment', got '%s'", payChild.Name)
	}

	// validate_cart deve ter 1 filho: check_stock
	if len(validateChild.Events) != 1 {
		t.Fatalf("expected 1 child event under validate_cart, got %d", len(validateChild.Events))
	}

	stockChild := validateChild.Events[0]
	if stockChild.Name != "check_stock" {
		t.Errorf("expected grandchild name 'check_stock', got '%s'", stockChild.Name)
	}

	// O total de eventos na árvore deve ser 4
	totalEvents := countAllEvents(processedLog.Events)
	if totalEvents != 4 {
		t.Errorf("expected 4 total events in tree, got %d", totalEvents)
	}
}

func TestStartEventWithoutCollectionInContext(t *testing.T) {
	// Sem Collection no contexto, StartEvent não deve causar panic
	ctx := context.Background()
	newCtx, eb := oltgo.StartEvent(ctx, "orphan_event", oltgo.TypeFunction, oltgo.SeverityInfo)

	// O contexto retornado deve ser o original
	if newCtx != ctx {
		t.Errorf("expected original context when no collection is present")
	}

	// O End() deve ser seguro e não causar panic
	eb.WithMessage("test").End()
}

func TestActiveEventContextPropagation(t *testing.T) {
	// Validar que WithActiveEvent e ActiveEventFromContext funcionam corretamente
	ctx := context.Background()

	// Sem evento ativo, deve retornar string vazia
	activeID := oltgo.ActiveEventFromContext(ctx)
	if activeID != "" {
		t.Errorf("expected empty active event ID, got '%s'", activeID)
	}

	// Injetar evento ativo
	ctx = oltgo.WithActiveEvent(ctx, "evt_001")
	activeID = oltgo.ActiveEventFromContext(ctx)
	if activeID != "evt_001" {
		t.Errorf("expected active event ID 'evt_001', got '%s'", activeID)
	}

	// Sobrescrever com outro evento ativo
	ctx = oltgo.WithActiveEvent(ctx, "evt_002")
	activeID = oltgo.ActiveEventFromContext(ctx)
	if activeID != "evt_002" {
		t.Errorf("expected active event ID 'evt_002', got '%s'", activeID)
	}
}
