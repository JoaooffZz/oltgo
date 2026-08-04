package tests

import (
	"testing"
	"time"

	"github.com/joaooffzz/oltgo"
)

// buildTrace roda fn sobre uma Collection e devolve o log comitado.
func buildTrace(t *testing.T, fn func(col *oltgo.Collection)) oltgo.LogSchema {
	t.Helper()
	var got oltgo.LogSchema
	done := make(chan struct{})
	agent := oltgo.NewAgent(
		oltgo.Service{Name: "test", Version: "1"}, oltgo.Testing, "1",
		func(l oltgo.LogSchema) error { got = l; close(done); return nil },
	)
	col := agent.NewCollection("trace_coherence")
	fn(col)
	col.Commit()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout aguardando ProcessLog")
	}
	agent.Close()
	return got
}

func TestIsFailureSeverity(t *testing.T) {
	cases := []struct {
		sev  oltgo.Severity
		want bool
	}{
		{oltgo.SeverityDebug, false},
		{oltgo.SeverityInfo, false},
		{oltgo.SeverityWarn, false},
		{oltgo.SeverityError, true},
		{oltgo.SeverityFatal, true},
		{oltgo.Severity("DESCONHECIDA"), false},
	}
	for _, tc := range cases {
		if got := oltgo.IsFailureSeverity(tc.sev); got != tc.want {
			t.Errorf("IsFailureSeverity(%q) = %v, esperado %v", tc.sev, got, tc.want)
		}
	}
}

// Severidade de falha implica status de falha, no evento e no trace.
func TestSeverityDerivesStatus(t *testing.T) {
	cases := []struct {
		sev        oltgo.Severity
		wantStatus oltgo.Status
	}{
		{oltgo.SeverityDebug, oltgo.StatusSuccess},
		{oltgo.SeverityInfo, oltgo.StatusSuccess},
		// "retry teve sucesso na segunda tentativa" é WARN + SUCCESS legítimo.
		{oltgo.SeverityWarn, oltgo.StatusSuccess},
		{oltgo.SeverityError, oltgo.StatusFailure},
		{oltgo.SeverityFatal, oltgo.StatusFailure},
	}

	for _, tc := range cases {
		t.Run(string(tc.sev), func(t *testing.T) {
			log := buildTrace(t, func(col *oltgo.Collection) {
				col.StartEvent("op", oltgo.TypeFunction, tc.sev).End()
			})
			if got := log.Events[0].Status; got != tc.wantStatus {
				t.Errorf("event.status = %s, esperado %s", got, tc.wantStatus)
			}
			if got := log.Status; got != tc.wantStatus {
				t.Errorf("trace.status = %s, esperado %s", got, tc.wantStatus)
			}
		})
	}
}

// AddError eleva a severidade para no mínimo ERROR, sem rebaixar FATAL.
func TestAddErrorRaisesSeverity(t *testing.T) {
	log := buildTrace(t, func(col *oltgo.Collection) {
		col.StartEvent("baixa", oltgo.TypeFunction, oltgo.SeverityInfo).
			AddError("BOOM", "explodiu", nil).End()
		col.StartEvent("alta", oltgo.TypeFunction, oltgo.SeverityFatal).
			AddError("BOOM", "explodiu", nil).End()
	})

	if got := log.Events[0].Severity; got != oltgo.SeverityError {
		t.Errorf("severity de evento INFO com erro = %s, esperado ERROR", got)
	}
	if got := log.Events[0].Status; got != oltgo.StatusFailure {
		t.Errorf("status = %s, esperado FAILURE", got)
	}
	if got := log.Events[1].Severity; got != oltgo.SeverityFatal {
		t.Errorf("severity de evento FATAL com erro = %s, esperado FATAL (sem rebaixar)", got)
	}
}

// Fail() marca falha sem exigir um objeto de erro.
func TestFailMarksFailureWithoutError(t *testing.T) {
	log := buildTrace(t, func(col *oltgo.Collection) {
		col.StartEvent("validacao", oltgo.TypeFunction, oltgo.SeverityWarn).
			WithMessage("payload rejeitado").Fail().End()
	})

	evt := log.Events[0]
	if evt.Status != oltgo.StatusFailure {
		t.Errorf("event.status = %s, esperado FAILURE", evt.Status)
	}
	if len(evt.Errors) != 0 {
		t.Errorf("esperado nenhum erro registrado, got %d", len(evt.Errors))
	}
	if evt.Severity != oltgo.SeverityWarn {
		t.Errorf("Fail() não deve alterar a severidade, got %s", evt.Severity)
	}
	if log.Status != oltgo.StatusFailure {
		t.Errorf("trace.status = %s, esperado FAILURE", log.Status)
	}
}

// Succeed() é a válvula de escape: erro tratado, execução segue por outro
// caminho. O override precisa valer também no status do trace.
func TestSucceedOverridesSeverityDerivation(t *testing.T) {
	log := buildTrace(t, func(col *oltgo.Collection) {
		col.StartEvent("cache_miss", oltgo.TypeDatabase, oltgo.SeverityError).
			WithMessage("cache indisponível, caiu no banco").Succeed().End()
	})

	evt := log.Events[0]
	if evt.Status != oltgo.StatusSuccess {
		t.Errorf("event.status = %s, esperado SUCCESS", evt.Status)
	}
	if evt.Severity != oltgo.SeverityError {
		t.Errorf("severity = %s, esperado ERROR preservado para diagnóstico", evt.Severity)
	}
	if log.Status != oltgo.StatusSuccess {
		t.Errorf("trace.status = %s, esperado SUCCESS — Succeed() foi anulado no roll-up", log.Status)
	}
}

// Um trace inteiro de eventos FATAL não pode ser reportado como SUCCESS,
// nem mesmo com HTTP 200.
func TestFatalEventsMakeTraceFailureDespiteHTTP200(t *testing.T) {
	log := buildTrace(t, func(col *oltgo.Collection) {
		col.SetRequest(&oltgo.Request{
			Communication: oltgo.ProtoREST,
			Method:        "GET",
			Route:         "/health",
			Status:        200,
		})
		for i := 0; i < 5; i++ {
			col.StartEvent("morreu", oltgo.TypeFunction, oltgo.SeverityFatal).
				WithMessage("serviço morreu").End()
		}
	})

	if log.Status != oltgo.StatusFailure {
		t.Errorf("trace.status = %s, esperado FAILURE com 5 eventos FATAL", log.Status)
	}
	for i, evt := range log.Events {
		if evt.Status != oltgo.StatusFailure {
			t.Errorf("evento %d: status = %s, esperado FAILURE", i, evt.Status)
		}
	}
}

// AddEvent deriva o status da severidade só quando Status não é informado.
func TestAddEventDerivesStatusOnlyWhenUnset(t *testing.T) {
	log := buildTrace(t, func(col *oltgo.Collection) {
		// Status não informado (zero value "") + FATAL → FAILURE.
		col.AddEvent(oltgo.Event{
			EventID: "e1", Name: "derivado", Type: oltgo.TypeFunction,
			Severity: oltgo.SeverityFatal, Timestamp: time.Now(),
		})
		// Status explícito SUCCESS é respeitado mesmo com severidade FATAL.
		col.AddEvent(oltgo.Event{
			EventID: "e2", Name: "explicito", Type: oltgo.TypeFunction,
			Status: oltgo.StatusSuccess, Severity: oltgo.SeverityFatal,
			Timestamp: time.Now(),
		})
		// Severidade baixa e status ausente → SUCCESS.
		col.AddEvent(oltgo.Event{
			EventID: "e3", Name: "ok", Type: oltgo.TypeFunction,
			Severity: oltgo.SeverityInfo, Timestamp: time.Now(),
		})
	})

	if got := log.Events[0].Status; got != oltgo.StatusFailure {
		t.Errorf("evento FATAL sem status = %s, esperado FAILURE", got)
	}
	if got := log.Events[1].Status; got != oltgo.StatusSuccess {
		t.Errorf("evento FATAL com SUCCESS explícito = %s, esperado SUCCESS respeitado", got)
	}
	if got := log.Events[2].Status; got != oltgo.StatusSuccess {
		t.Errorf("evento INFO sem status = %s, esperado SUCCESS", got)
	}
	if log.Status != oltgo.StatusFailure {
		t.Errorf("trace.status = %s, esperado FAILURE", log.Status)
	}
}

// Um evento com Errors inserido por AddEvent é falha e nunca fica abaixo de ERROR.
func TestAddEventWithErrorsForcesFailure(t *testing.T) {
	log := buildTrace(t, func(col *oltgo.Collection) {
		col.AddEvent(oltgo.Event{
			EventID: "e1", Name: "com_erro", Type: oltgo.TypeFunction,
			Status: oltgo.StatusSuccess, Severity: oltgo.SeverityInfo,
			Timestamp: time.Now(),
			Errors:    []oltgo.EventError{{Code: "BOOM", Message: "explodiu"}},
		})
	})

	evt := log.Events[0]
	if evt.Status != oltgo.StatusFailure {
		t.Errorf("status = %s, esperado FAILURE", evt.Status)
	}
	if evt.Severity != oltgo.SeverityError {
		t.Errorf("severity = %s, esperado ERROR", evt.Severity)
	}
	if log.Status != oltgo.StatusFailure {
		t.Errorf("trace.status = %s, esperado FAILURE", log.Status)
	}
}
