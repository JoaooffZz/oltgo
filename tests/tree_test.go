package tests

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/joaooffzz/oltgo"
)

func countAll(evs []*oltgo.Event) int {
	n := 0
	for _, e := range evs {
		n += 1 + countAll(e.Events)
	}
	return n
}

// commit monta uma Collection com os eventos dados e devolve o log resultante.
func commit(t *testing.T, events []oltgo.Event) oltgo.LogSchema {
	t.Helper()
	var got oltgo.LogSchema
	done := make(chan struct{})
	agent := oltgo.NewAgent(
		oltgo.Service{Name: "test", Version: "1"}, oltgo.Testing, "1",
		func(l oltgo.LogSchema) error { got = l; close(done); return nil },
	)
	col := agent.NewCollection("trace_test")
	for _, e := range events {
		col.AddEvent(e)
	}
	col.Commit()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout aguardando ProcessLog")
	}
	agent.Close()
	return got
}

func evt(id string, parent *string, name string) oltgo.Event {
	return oltgo.Event{
		EventID: id, ParentEventID: parent, Name: name,
		Type: oltgo.TypeFunction, Status: oltgo.StatusSuccess,
		Severity: oltgo.SeverityInfo, Timestamp: time.Now(),
	}
}

func ptr(s string) *string { return &s }

// collectNames devolve o multiconjunto de nomes presentes na árvore.
func collectNames(evs []*oltgo.Event) map[string]int {
	out := map[string]int{}
	var walk func([]*oltgo.Event)
	walk = func(list []*oltgo.Event) {
		for _, e := range list {
			out[e.Name]++
			walk(e.Events)
		}
	}
	walk(evs)
	return out
}

func TestTreeNoEventIsLost(t *testing.T) {
	cases := []struct {
		name  string
		in    []oltgo.Event
		total int
	}{
		{"árvore normal", []oltgo.Event{
			evt("evt_001", nil, "root"),
			evt("evt_002", ptr("evt_001"), "child"),
			evt("evt_003", ptr("evt_002"), "grandchild"),
		}, 3},
		{"event_id duplicado", []oltgo.Event{
			evt("dup", nil, "FIRST"),
			evt("dup", nil, "SECOND"),
		}, 2},
		{"duplicado com filhos", []oltgo.Event{
			evt("dup", nil, "FIRST"),
			evt("dup", nil, "SECOND"),
			evt("kid", ptr("dup"), "kid"),
		}, 3},
		{"órfão", []oltgo.Event{
			evt("evt_001", ptr("inexistente"), "orfao"),
		}, 1},
		{"ciclo de 2", []oltgo.Event{
			evt("A", ptr("B"), "A"),
			evt("B", ptr("A"), "B"),
		}, 2},
		{"auto-referência", []oltgo.Event{
			evt("A", ptr("A"), "A"),
		}, 1},
		{"ciclo de 3", []oltgo.Event{
			evt("A", ptr("C"), "A"),
			evt("B", ptr("A"), "B"),
			evt("C", ptr("B"), "C"),
		}, 3},
		{"parent vazio é raiz", []oltgo.Event{
			evt("evt_001", ptr(""), "raiz"),
		}, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			log := commit(t, tc.in)
			if got := countAll(log.Events); got != tc.total {
				t.Errorf("eventos na árvore = %d, esperado %d", got, tc.total)
			}
			// A contagem sozinha não detecta substituição: dois eventos de mesmo
			// id colapsados em duas cópias do segundo também somam 2. Comparar o
			// multiconjunto de nomes é o que prova que nada foi trocado.
			want := map[string]int{}
			for _, e := range tc.in {
				want[e.Name]++
			}
			got := collectNames(log.Events)
			for name, n := range want {
				if got[name] != n {
					t.Errorf("nome %q aparece %d vez(es), esperado %d", name, got[name], n)
				}
			}
			for name, n := range got {
				if want[name] != n {
					t.Errorf("nome %q aparece %d vez(es) na saída, esperado %d", name, n, want[name])
				}
			}
			// A serialização precisa terminar: um ciclo não quebrado recursaria
			// infinitamente aqui.
			if _, err := json.Marshal(log); err != nil {
				t.Errorf("json.Marshal falhou: %v", err)
			}
		})
	}
}

func TestTimestampsAreUTC(t *testing.T) {
	log := commit(t, []oltgo.Event{evt("evt_001", nil, "root")})

	if loc := log.Time.CreatedAt.Location(); loc != time.UTC {
		t.Errorf("created_at em %v, esperado UTC", loc)
	}
	if loc := log.Time.FinishedAt.Location(); loc != time.UTC {
		t.Errorf("finished_at em %v, esperado UTC", loc)
	}
	if loc := log.Events[0].Timestamp.Location(); loc != time.UTC {
		t.Errorf("event timestamp em %v, esperado UTC", loc)
	}

	b, _ := json.Marshal(log.Time.CreatedAt)
	if s := string(b); s[len(s)-2] != 'Z' {
		t.Errorf("created_at serializado como %s, esperado sufixo Z", s)
	}
}

// A duração precisa continuar vindo do relógio monotônico após a mudança para
// UTC. Um evento de ~50ms medido pelo relógio de parede pode dar qualquer coisa
// se houver ajuste de NTP; medido pelo monotônico, não.
func TestDurationSurvivesUTCConversion(t *testing.T) {
	var got oltgo.LogSchema
	done := make(chan struct{})
	agent := oltgo.NewAgent(
		oltgo.Service{Name: "test", Version: "1"}, oltgo.Testing, "1",
		func(l oltgo.LogSchema) error { got = l; close(done); return nil },
	)
	col := agent.NewCollection("trace_dur")
	e := col.StartEvent("slow", oltgo.TypeFunction, oltgo.SeverityInfo)
	time.Sleep(50 * time.Millisecond)
	e.End()
	col.Commit()
	<-done
	agent.Close()

	if d := got.Events[0].DurationMs; d < 45 || d > 200 {
		t.Errorf("duration_ms = %d, esperado ~50", d)
	}
}

func TestCommitIsIdempotent(t *testing.T) {
	var count int
	agent := oltgo.NewAgent(
		oltgo.Service{Name: "test", Version: "1"}, oltgo.Testing, "1",
		func(l oltgo.LogSchema) error { count++; return nil },
	)
	col := agent.NewCollection("trace_dup_commit")
	col.AddEvent(evt("evt_001", nil, "root"))
	col.Commit()
	col.Commit()
	col.Commit()
	agent.Close() // drena a fila

	if count != 1 {
		t.Errorf("ProcessLog chamado %d vezes, esperado 1", count)
	}
}

func TestShippingCountsDrops(t *testing.T) {
	// Buffer de 1 com ProcessLog lento: os Ship seguintes devem ser descartados.
	block := make(chan struct{})
	agent := oltgo.NewAgentWithOptions(
		oltgo.Service{Name: "test", Version: "1"}, oltgo.Testing, "1",
		func(l oltgo.LogSchema) error { <-block; return nil },
		oltgo.Options{BufferSize: 1},
	)
	for i := 0; i < 50; i++ {
		col := agent.NewCollection("t")
		col.Commit()
	}
	if d := agent.Stats().Dropped; d == 0 {
		t.Error("Dropped = 0, esperado > 0 com buffer saturado")
	}
	close(block)
	agent.Close()
}

func TestShippingCountsFailuresAndInvokesOnError(t *testing.T) {
	var mu sync.Mutex
	var seen []error
	agent := oltgo.NewAgentWithOptions(
		oltgo.Service{Name: "test", Version: "1"}, oltgo.Testing, "1",
		func(l oltgo.LogSchema) error { return errors.New("falha no envio") },
		oltgo.Options{OnError: func(l oltgo.LogSchema, err error) {
			mu.Lock()
			defer mu.Unlock()
			seen = append(seen, err)
		}},
	)
	agent.NewCollection("t1").Commit()
	agent.NewCollection("t2").Commit()
	agent.Close()

	st := agent.Stats()
	if st.Failed != 2 {
		t.Errorf("Failed = %d, esperado 2", st.Failed)
	}
	if st.Shipped != 0 {
		t.Errorf("Shipped = %d, esperado 0", st.Shipped)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Errorf("OnError chamado %d vez(es), esperado 2", len(seen))
	}
}

func TestShippingCountsShippedAndDropsAfterClose(t *testing.T) {
	agent := oltgo.NewAgent(
		oltgo.Service{Name: "test", Version: "1"}, oltgo.Testing, "1",
		func(l oltgo.LogSchema) error { return nil },
	)
	agent.NewCollection("t1").Commit()
	agent.Close()

	if s := agent.Stats().Shipped; s != 1 {
		t.Errorf("Shipped = %d, esperado 1", s)
	}

	// Ship após Close() é telemetria perdida: precisa aparecer no contador.
	agent.NewCollection("depois-do-close").Commit()
	if d := agent.Stats().Dropped; d != 1 {
		t.Errorf("Dropped = %d após Ship pós-Close, esperado 1", d)
	}
}

func TestRootEventOmitsParentField(t *testing.T) {
	log := commit(t, []oltgo.Event{evt("evt_001", nil, "root")})
	b, _ := json.Marshal(log.Events[0])
	var m map[string]any
	json.Unmarshal(b, &m)

	if _, present := m["parent_event_id"]; present {
		t.Errorf("parent_event_id presente em evento raiz: %s", b)
	}
}
