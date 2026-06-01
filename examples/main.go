package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/joaooffzz/oltgo"
)

func main() {
	// 1. Inicializa o serviço com metadados básicos
	serviceName := "order-api"
	serviceDesc := "API responsável pelo checkout e processamento de pedidos"
	serviceTags := []string{"ecommerce", "checkout"}
	
	service := oltgo.Service{
		Name:        serviceName,
		Description: &serviceDesc,
		Version:     "1.0.0",
		Tags:        &serviceTags,
	}

	// 2. Define a função ProcessLog para processamento customizado (ex: imprimir JSON formatado)
	processLog := func(log oltgo.LogSchema) error {
		data, err := json.MarshalIndent(log, "", "  ")
		if err != nil {
			return err
		}
		fmt.Printf("\n--- [NOVO LOG RECEBIDO ASSINCRONAMENTE] ---\n%s\n--------------------------------------------\n", string(data))
		return nil
	}

	// 3. Inicializa o Agente de Observabilidade (1 Agent por Serviço)
	agent := oltgo.NewAgent(service, oltgo.Development, "1", processLog)
	defer agent.Close() // Garante o flush final dos logs em buffer

	// 4. Configura as rotas do servidor HTTP
	mux := http.NewServeMux()
	mux.HandleFunc("/checkout", func(w http.ResponseWriter, r *http.Request) {
		// Middleware manual de injeção de Collection no contexto
		tracingID := fmt.Sprintf("trace_%d", time.Now().UnixNano())
		collection := agent.NewCollection(tracingID)
		
		// Registra informações da requisição recebida
		collection.SetRequestFromHTTP(r, oltgo.ProtoREST, http.StatusOK)
		
		// Configura o ator
		collection.SetActor(&oltgo.Actor{
			ID:   "usr_guest_456",
			Type: "USER",
			IP:   r.RemoteAddr,
		})

		// Coloca a coleção no contexto e chama a lógica de negócio
		ctx := oltgo.WithCollection(r.Context(), collection)
		
		err := checkoutHandler(ctx)
		if err != nil {
			collection.SetRequest(&oltgo.Request{
				Communication: oltgo.ProtoREST,
				Method:        r.Method,
				Route:         r.URL.Path,
				Status:        http.StatusInternalServerError,
				UserAgent:     r.UserAgent(),
			})
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("checkout failed"))
		} else {
			w.Write([]byte("checkout success"))
		}

		// Commita a coleção ao finalizar a requisição
		collection.Commit()
	})

	fmt.Println("Servidor de exemplo rodando em http://localhost:8080")
	fmt.Println("Tente fazer uma requisição: curl http://localhost:8080/checkout")
	
	// Roda o servidor de exemplo em background por alguns segundos ou indefinitivamente
	server := &http.Server{Addr: ":8080", Handler: mux}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Erro no servidor: %v\n", err)
		}
	}()

	// Simula chamadas locais automáticas para demonstrar o log sem precisar de curl externo
	time.Sleep(1 * time.Second)
	fmt.Println("\nSimulando requisição local de sucesso...")
	simulateLocalCall(agent, false)

	time.Sleep(1 * time.Second)
	fmt.Println("\nSimulando requisição local com falha...")
	simulateLocalCall(agent, true)

	time.Sleep(1 * time.Second)
	fmt.Println("Desligando servidor de exemplo...")
	server.Shutdown(context.Background())
}

// Lógica de negócios simulada que demonstra o uso do context.Context
func checkoutHandler(ctx context.Context) error {
	// Recupera a Collection propagada no contexto
	col := oltgo.FromContext(ctx)

	// Evento 1: Autenticação
	event1 := col.StartEvent("authenticate_user", oltgo.TypeFunction, oltgo.SeverityInfo)
	time.Sleep(15 * time.Millisecond) // Simula processamento
	event1.WithMessage("Usuário autenticado com JWT local").End()

	// Evento 2: Query no Banco de Dados
	event2 := col.StartEvent("fetch_items", oltgo.TypeDatabase, oltgo.SeverityInfo)
	time.Sleep(45 * time.Millisecond) // Simula query
	event2.WithMessage("Consulta de catálogo finalizada").
		WithMetadata(map[string]any{
			"table":          "products",
			"rows_returned":  3,
			"execution_time": "45ms",
		}).End()

	return nil
}

// Simulador local para verificação direta no terminal
func simulateLocalCall(agent *oltgo.Agent, simulateFailure bool) {
	tracingID := fmt.Sprintf("trace_%d", rand.Intn(999999))
	col := agent.NewCollection(tracingID)

	col.SetRequest(&oltgo.Request{
		Communication: oltgo.ProtoREST,
		Method:        "POST",
		Route:         "/v1/orders/checkout",
		Status:        200,
		UserAgent:     "SimulatedClient/1.0",
	})

	col.SetActor(&oltgo.Actor{
		ID:   "usr_simulated_123",
		Type: "USER",
		IP:   "192.168.1.100",
	})

	// Rastreando uma função interna
	evtFunc := col.StartEvent("process_payment", oltgo.TypeExternalService, oltgo.SeverityInfo)
	time.Sleep(30 * time.Millisecond)

	if simulateFailure {
		stackTrace := "PaymentError: Insufficient funds\n  at stripe.go:42\n  at main.go:120"
		evtFunc.AddError("PAYMENT_DECLINED", "Saldo insuficiente no cartão de crédito", &stackTrace)
		evtFunc.WithMessage("Falha no pagamento externo com o Stripe")
		col.SetRequest(&oltgo.Request{
			Communication: oltgo.ProtoREST,
			Method:        "POST",
			Route:         "/v1/orders/checkout",
			Status:        402,
			UserAgent:     "SimulatedClient/1.0",
		})
	} else {
		evtFunc.WithMessage("Pagamento aprovado")
	}
	evtFunc.End()

	col.Commit()
}
