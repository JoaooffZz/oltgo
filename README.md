# Oltgo — Biblioteca de Telemetria e Observabilidade em Go

O `oltgo` é uma biblioteca leve, rápida e concorrente projetada para capturar e estruturar logs e rastreamentos (traces) em microsserviços, monólitos ou quaisquer aplicações escritas em Go.

---

## 1. Arquitetura Geral

O design segue o princípio **1 Serviço -> 1 Agente**, onde cada instância da sua aplicação mantém um único `Agent` em execução.

```
+-------------------------------------------------------------+
|                     Aplicação Go                            |
|                                                             |
|   +------------------+           +----------------------+   |
|   |    Collection    | --------> |       Shipping       |   |
|   | (Fila local/Mem) |  (Canal)  | (Worker assíncrono)  |   |
|   +------------------+           +----------------------+   |
+---------------------------------------------|---------------+
                                              v
                                   [ Callback ProcessLog ]
                                   (ex: Gravar em arquivo, 
                                    enviar p/ OpenTelemetry, 
                                    Stripe, ElasticSearch, etc)
```

### Componentes Principais

1. **`Agent`**: Orquestrador central que gerencia o ciclo de vida do pipeline. É inicializado uma única vez na inicialização do serviço.
2. **`Collection`**: Estrutura *thread-safe* criada para cada transação, requisição ou trace. É responsável por acumular os dados e eventos em memória local e empacotá-los sob o schema unificado ao final da execução.
3. **`Shipping`**: Fila de buffer baseada em *Go channels*. Ela recebe as instâncias estruturadas prontas do `Collection` e consome assincronamente os dados para que a aplicação principal nunca sofra atrasos de I/O de escrita.

---

## 2. Inicializando o Agente

Para começar, inicialize o `Agent` definindo os metadados do serviço e um callback opcional de destino para os logs:

```go
package main

import (
	"encoding/json"
	"fmt"
	"github.com/joaooffzz/oltgo"
)

func main() {
	service := oltgo.Service{
		Name:    "payment-gateway",
		Version: "2.1.0",
	}

	// Callback para exportar logs estruturados (ex: saída padrão stdout)
	processLog := func(log oltgo.LogSchema) error {
		bytes, _ := json.Marshal(log)
		fmt.Println(string(bytes))
		return nil
	}

	// Inicializa
	agent := oltgo.NewAgent(service, oltgo.Production, "1", processLog)
	
	// Garante que todos os logs na fila de buffer sejam processados antes da app fechar
	defer agent.Close() 
}
```

---

## 3. Propagação de Contexto via `context.Context`

O `oltgo` utiliza o `context.Context` nativo do Go para carregar e obter a coleção de telemetria através da pilha de execução de funções (*call stack*), evitando o acoplamento excessivo de código e garantindo segurança concorrente.

### Criação e Injeção do Contexto

No início de um fluxo de requisição (por exemplo, um middleware HTTP), você cria a coleção e injeta-a no contexto:

```go
func HandleRequest(w http.ResponseWriter, r *http.Request) {
	// Cria a collection para esta chamada específica
	collection := agent.NewCollection("trace-unique-uuid")
	collection.SetRequestFromHTTP(r, oltgo.ProtoREST, 200)

	// Injeta a collection no contexto
	ctx := oltgo.WithCollection(r.Context(), collection)

	// Propaga o contexto modificado para as funções internas
	executeBusinessLogic(ctx)

	// Commita os dados para a fila de envio assíncrono
	collection.Commit()
}
```

### Extração e Registro de Eventos na Pilha

Em qualquer sub-função que receba o contexto, você pode extrair a `Collection` com segurança e registrar eventos:

```go
func executeBusinessLogic(ctx context.Context) {
	// Recupera a coleção. Se não estiver no contexto, retorna nil com segurança
	collection := oltgo.FromContext(ctx)
	if collection == nil {
		return
	}

	// Inicia um evento com cálculo de tempo automático
	event := collection.StartEvent("db_query", oltgo.TypeDatabase, oltgo.SeverityInfo)
	event.WithMessage("Selecionando itens do carrinho")
	
	defer event.End() // Ao finalizar a função, calcula a duração e salva o evento

	// Executa operação de fato...
	if err := queryDB(); err != nil {
		event.AddError("DB_ERROR", err.Error(), nil)
	}
}
```

---

## 4. O Schema de Log Estruturado

Todos os logs commitados são unificados sob uma estrutura padrão e enviados para o callback `ProcessLog`. A documentação completa dos campos do schema JSON pode ser vista em:
- [Documentação de Campos (docs/log.md)](file:///Users/macbook/projects/oltgo/docs/log.md)
