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

	// 3.1. Registra a inicialização do serviço — um trace sem "request"
	fmt.Println("\n--- Simulando log de inicialização (sem request) ---")
	simulateStartupLog(agent)
	time.Sleep(1 * time.Second)

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
	fmt.Println("\n--- Simulando fluxo com eventos hierárquicos (context.Context) ---")
	simulateHierarchicalFlow(agent)

	time.Sleep(1 * time.Second)
	fmt.Println("Desligando servidor de exemplo...")
	server.Shutdown(context.Background())
}

// simulateStartupLog demonstra um trace de inicialização: como não há requisição
// envolvida, SetRequest nunca é chamado e o campo "request" é omitido do JSON.
func simulateStartupLog(agent *oltgo.Agent) {
	col := agent.NewCollection(fmt.Sprintf("trace_startup_%d", time.Now().UnixNano()))
	ctx := oltgo.WithCollection(context.Background(), col)

	ctx, bootEvt := oltgo.StartEvent(ctx, "bootstrap", oltgo.TypeFunction, oltgo.SeverityInfo)

	// Subevento: carregamento de configuração
	_, cfgEvt := oltgo.StartEvent(ctx, "load_config", oltgo.TypeFunction, oltgo.SeverityInfo)
	time.Sleep(8 * time.Millisecond)
	cfgEvt.WithMessage("Variáveis de ambiente carregadas").
		WithMetadata(map[string]any{
			"source": "env",
			"keys":   []string{"DATABASE_URL", "STRIPE_KEY", "PORT"},
		}).End()

	// Subevento: conexão com o banco de dados
	_, dbEvt := oltgo.StartEvent(ctx, "connect_database", oltgo.TypeDatabase, oltgo.SeverityInfo)
	time.Sleep(35 * time.Millisecond)
	dbEvt.WithMessage("Pool de conexões estabelecido").
		WithMetadata(map[string]any{
			"driver":         "postgres",
			"max_open_conns": 25,
		}).End()

	bootEvt.WithMessage("Serviço inicializado com sucesso").End()

	col.Commit()
}

// Lógica de negócios simulada que demonstra o uso do context.Context
// com a nova API de eventos hierárquicos (oltgo.StartEvent)
func checkoutHandler(ctx context.Context) error {
	// Evento raiz: checkout_flow — todos os subeventos serão filhos dele
	ctx, mainEvt := oltgo.StartEvent(ctx, "checkout_flow", oltgo.TypeFunction, oltgo.SeverityInfo)
	defer mainEvt.WithMessage("Fluxo de checkout completo").End()

	// Subevento: Autenticação (filho de checkout_flow)
	authenticateUser(ctx)

	// Subevento: Busca de itens (filho de checkout_flow)
	fetchItems(ctx)

	return nil
}

// authenticateUser demonstra um subevento simples que herda o pai do contexto
func authenticateUser(ctx context.Context) {
	_, evt := oltgo.StartEvent(ctx, "authenticate_user", oltgo.TypeFunction, oltgo.SeverityInfo)
	time.Sleep(15 * time.Millisecond)
	evt.WithMessage("Usuário autenticado com JWT local").End()
}

// fetchItems demonstra um subevento com metadata que herda o pai do contexto
func fetchItems(ctx context.Context) {
	_, evt := oltgo.StartEvent(ctx, "fetch_items", oltgo.TypeDatabase, oltgo.SeverityInfo)
	time.Sleep(45 * time.Millisecond)
	evt.WithMessage("Consulta de catálogo finalizada").
		WithMetadata(map[string]any{
			"table":          "products",
			"rows_returned":  3,
			"execution_time": "45ms",
		}).End()
}

// Simulador local para verificação direta no terminal (usa a API antiga col.StartEvent)
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

// simulateHierarchicalFlow demonstra a nova funcionalidade de eventos hierárquicos
// usando context.Context para propagar automaticamente o parent_event_id
func simulateHierarchicalFlow(agent *oltgo.Agent) {
	tracingID := fmt.Sprintf("trace_hierarchy_%d", rand.Intn(999999))
	col := agent.NewCollection(tracingID)

	col.SetRequest(&oltgo.Request{
		Communication: oltgo.ProtoREST,
		Method:        "POST",
		Route:         "/v1/orders/process",
		Status:        200,
		UserAgent:     "SimulatedClient/2.0",
	})

	col.SetActor(&oltgo.Actor{
		ID:   "usr_premium_789",
		Type: "USER",
		IP:   "10.0.0.50",
	})

	// Injeta a Collection no contexto
	ctx := oltgo.WithCollection(context.Background(), col)

	// ===== ÁRVORE DE EVENTOS =====
	// process_order (raiz)
	//   ├── validate_cart
	//   │     ├── check_stock
	//   │     └── calculate_shipping
	//   ├── authorize_payment
	//   │     └── call_stripe_api
	//   └── send_confirmation_email

	// Nível 0: Evento raiz
	ctx, processEvt := oltgo.StartEvent(ctx, "process_order", oltgo.TypeFunction, oltgo.SeverityInfo)
	processEvt.WithMessage("Processando pedido #ORD-2026-0042")

	// Nível 1: validate_cart (filho de process_order)
	ctxValidate, validateEvt := oltgo.StartEvent(ctx, "validate_cart", oltgo.TypeFunction, oltgo.SeverityInfo)
	time.Sleep(5 * time.Millisecond)

	// Nível 2: check_stock (filho de validate_cart)
	_, stockEvt := oltgo.StartEvent(ctxValidate, "check_stock", oltgo.TypeDatabase, oltgo.SeverityInfo)
	time.Sleep(20 * time.Millisecond)
	stockEvt.WithMessage("Estoque verificado: 3 itens disponíveis").
		WithMetadata(map[string]any{
			"warehouse": "SP-01",
			"items":     []string{"SKU-001", "SKU-042", "SKU-100"},
		}).End()

	// Nível 2: calculate_shipping (filho de validate_cart)
	_, shipEvt := oltgo.StartEvent(ctxValidate, "calculate_shipping", oltgo.TypeExternalService, oltgo.SeverityInfo)
	time.Sleep(15 * time.Millisecond)
	shipEvt.WithMessage("Frete calculado via Correios API").
		WithMetadata(map[string]any{
			"carrier":      "CORREIOS",
			"service":      "SEDEX",
			"cost_brl":     29.90,
			"estimated_days": 3,
		}).End()

	validateEvt.WithMessage("Carrinho validado com sucesso").End()

	// Nível 1: authorize_payment (filho de process_order)
	ctxPay, payEvt := oltgo.StartEvent(ctx, "authorize_payment", oltgo.TypeFunction, oltgo.SeverityInfo)
	time.Sleep(10 * time.Millisecond)

	// Nível 2: call_stripe_api (filho de authorize_payment)
	_, stripeEvt := oltgo.StartEvent(ctxPay, "call_stripe_api", oltgo.TypeExternalService, oltgo.SeverityInfo)
	time.Sleep(50 * time.Millisecond)
	stripeEvt.WithMessage("Pagamento autorizado com sucesso via Stripe").
		WithMetadata(map[string]any{
			"payment_intent": "pi_3abc123def456",
			"amount_cents":   15990,
			"currency":       "BRL",
			"card_last4":     "4242",
		}).End()

	payEvt.WithMessage("Pagamento autorizado").End()

	// Nível 1: send_confirmation_email (filho de process_order)
	_, emailEvt := oltgo.StartEvent(ctx, "send_confirmation_email", oltgo.TypeExternalService, oltgo.SeverityInfo)
	time.Sleep(25 * time.Millisecond)
	emailEvt.WithMessage("E-mail de confirmação enviado").
		WithMetadata(map[string]any{
			"to":       "cliente@email.com",
			"template": "order_confirmation_v2",
		}).End()

	processEvt.End()

	col.Commit()
}
