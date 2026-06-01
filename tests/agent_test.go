package tests

import (
	"context"
	"errors"
	"net/http"
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
		t.Errorf("expected 2 events, got %d", len(processedLog.Events))
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
		processedCount += len(log.Events)
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
