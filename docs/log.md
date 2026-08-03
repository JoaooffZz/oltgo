# Log Schema — Documentação de Campos

---

```json
{
  "tracing_id": "trace_4d2e8a1f-bc37-4f9e-a12c-7e3d5b90c841",
  "status": "FAILURE",
  "environment": "production",
  "schema_version": "1",
  "service": {
    "id": "instancia-1",
    "name": "Processamento de pedidos",
    "description": "Servico responsavel pelo processamento de pedidos",
    "version": "1.0.0",
    "tags": ["order"]
  },
  "request": {
    "communication": "REST",
    "method": "POST",
    "route": "/v1/orders/checkout",
    "status": 402,
    "user_agent": "Mozilla/5.0"
  },
  "actor": {
    "id": "123132",
    "type": "USER",
    "ip": "192.168.1.42"
  },
  "time": {
    "duration_ms": 695,
    "created_at": "2026-05-24T10:32:15.123Z",
    "finished_at": "2026-05-24T10:32:15.818Z"
  },
  "events": [

    {
      "event_id": "evt_001",
      "name": "authenticate_user",
      "type": "FUNCTION",
      "status": "SUCCESS",
      "severity": "INFO",
      "timestamp": "2026-05-24T10:32:15.123Z",
      "duration_ms": 8,
      "message": "Token JWT validado com sucesso",
      "metadata": {
        "method": "JWT",
        "user_id": "usr_9f3a1c",
        "token_expires_at": "2026-05-24T11:32:15.000Z"
      }
    },

    {
      "event_id": "evt_002",
      "name": "process_order",
      "type": "FUNCTION",
      "status": "FAILURE",
      "severity": "ERROR",
      "timestamp": "2026-05-24T10:32:15.131Z",
      "duration_ms": 687,
      "message": "Falha no processamento do pedido",
      "events": [

        {
          "event_id": "evt_003",
          "parent_event_id": "evt_002",
          "name": "validate_request_payload",
          "type": "FUNCTION",
          "status": "SUCCESS",
          "severity": "INFO",
          "timestamp": "2026-05-24T10:32:15.131Z",
          "duration_ms": 3,
          "message": "Payload da requisição validado com sucesso",
          "metadata": {
            "fields_validated": ["items", "shipping_address", "payment_method"],
            "items_count": 3
          }
        },

        {
          "event_id": "evt_004",
          "parent_event_id": "evt_002",
          "name": "fetch_cart_items",
          "type": "DATABASE",
          "status": "SUCCESS",
          "severity": "INFO",
          "timestamp": "2026-05-24T10:32:15.156Z",
          "duration_ms": 18,
          "message": "Itens do carrinho recuperados com sucesso",
          "metadata": {
            "table": "cart_items",
            "query": "SELECT * FROM cart_items WHERE user_id = $1 AND status = 'active'",
            "rows_returned": 3
          }
        },

        {
          "event_id": "evt_005",
          "parent_event_id": "evt_002",
          "name": "check_stock_availability",
          "type": "INTERNAL_SERVICE",
          "status": "SUCCESS",
          "severity": "INFO",
          "timestamp": "2026-05-24T10:32:15.174Z",
          "duration_ms": 31,
          "message": "Estoque verificado, todos os itens disponíveis",
          "metadata": {
            "service": "inventory-service",
            "topic": "stock.check",
            "items_checked": 3,
            "all_available": true
          }
        },

        {
          "event_id": "evt_006",
          "parent_event_id": "evt_002",
          "name": "process_payment",
          "type": "EXTERNAL_SERVICE",
          "status": "FAILURE",
          "severity": "ERROR",
          "timestamp": "2026-05-24T10:32:15.234Z",
          "duration_ms": 541,
          "message": "Falha ao processar pagamento — cartão recusado",
          "metadata": {
            "provider": "Stripe",
            "endpoint": "https://api.stripe.com/v1/charges",
            "http_status": 402,
            "attempt": 2,
            "max_attempts": 2,
            "payment_method": "credit_card",
            "card_last4": "4242",
            "amount": 363.36,
            "currency": "BRL"
          },
          "errors": [
            {
              "code": "CARD_DECLINED",
              "message": "Cartão recusado pela operadora",
              "stack": null
            }
          ],
          "events": [

            {
              "event_id": "evt_007",
              "parent_event_id": "evt_006",
              "name": "release_stock_reservation",
              "type": "INTERNAL_SERVICE",
              "status": "SUCCESS",
              "severity": "WARN",
              "timestamp": "2026-05-24T10:32:15.775Z",
              "duration_ms": 19,
              "message": "Reserva de estoque liberada após falha no pagamento",
              "metadata": {
                "service": "inventory-service",
                "topic": "stock.release",
                "reservation_id": "rsv_3c7e2a"
              }
            },

            {
              "event_id": "evt_008",
              "parent_event_id": "evt_006",
              "name": "notify_user_payment_failed",
              "type": "INTERNAL_SERVICE",
              "status": "SUCCESS",
              "severity": "WARN",
              "timestamp": "2026-05-24T10:32:15.794Z",
              "duration_ms": 14,
              "message": "Notificação de falha no pagamento enviada ao usuário",
              "metadata": {
                "service": "notification-service",
                "topic": "payment.failed",
                "channels": ["email", "push"],
                "user_id": "usr_9f3a1c"
              }
            }

          ]
        }

      ]
    }

  ]
}
```


---

## Exemplo — Log de Inicialização (sem `request`)

O campo `request` é **opcional**. Traces que não nascem de uma requisição — inicialização do serviço, jobs agendados, workers de fila, CLIs — simplesmente não chamam `SetRequest` e o campo é omitido do JSON:

```json
{
  "tracing_id": "trace_startup_9f2a1c",
  "status": "SUCCESS",
  "environment": "production",
  "schema_version": "1",
  "service": {
    "name": "Processamento de pedidos",
    "version": "1.0.0"
  },
  "time": {
    "duration_ms": 412,
    "created_at": "2026-05-24T10:00:00.000Z",
    "finished_at": "2026-05-24T10:00:00.412Z"
  },
  "events": [
    {
      "event_id": "evt_001",
      "name": "bootstrap",
      "type": "FUNCTION",
      "status": "SUCCESS",
      "severity": "INFO",
      "timestamp": "2026-05-24T10:00:00.000Z",
      "duration_ms": 412,
      "message": "Serviço inicializado",
      "events": [
        {
          "event_id": "evt_002",
          "parent_event_id": "evt_001",
          "name": "connect_database",
          "type": "DATABASE",
          "status": "SUCCESS",
          "severity": "INFO",
          "timestamp": "2026-05-24T10:00:00.010Z",
          "duration_ms": 180,
          "message": "Pool de conexões estabelecido",
          "metadata": {
            "driver": "postgres",
            "max_open_conns": 25
          }
        }
      ]
    }
  ]
}
```

Note que `request` e `actor` estão ausentes — não são enviados como `null`, são omitidos.

---

## Envelope Raiz

| Campo            | Tipo        | Obrigatório | Descrição                                                                                                          | Exemplo                                        |
| ---------------- | ----------- | ----------- | ------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------- |
| `tracing_id`     | `string`    | ✅           | Identificador único global do trace                                                                                 | `"trace_4d2e8a1f-bc37-4f9e-a12c-7e3d5b90c841"` |
| `status`         | `enum`      | ✅           | Status final do trace. Valores: `SUCCESS`, `FAILURE`                                                               | `"FAILURE"`                                    |
| `environment`    | `enum`      | ✅           | Ambiente de execução                                                                                               | `"production"`, `"staging"`, `"development"`    |
| `schema_version` | `string`    | ✅           | Versão do schema do log. Permite evoluir o formato sem quebrar consumidores                                        | `"1"`                                          |
| `service`        | `object`    | ✅           | Serviço que gerou o log. Ver [`service`](#service)                                                                 | ver abaixo                                     |
| `request`        | `object`    | ❌           | Requisição que originou o trace. **Omitido** em traces sem requisição (inicialização, jobs, workers, CLIs)          | ver abaixo                                     |
| `actor`          | `object`    | ❌           | Quem disparou o trace. Omitido quando não há usuário ou serviço chamador                                           | ver abaixo                                     |
| `time`           | `object`    | ✅           | Temporização total do trace. Ver [`time`](#time)                                                                   | ver abaixo                                     |
| `events`         | `[]Event`   | ✅           | Eventos rastreados. Array vazio quando nada foi registrado                                                         | ver abaixo                                     |

---

## `service`

Informações sobre o serviço que gerou o log.

| Campo         | Tipo       | Obrigatório | Descrição                                | Exemplo                                               |
| ------------- | ---------- | ----------- | ---------------------------------------- | ----------------------------------------------------- |
| `id`          | `string`   | ❌           | Identificador da instância do serviço    | `"instancia-1"`                                       |
| `name`        | `string`   | ✅           | Nome legível do serviço                  | `"Processamento de pedidos"`                          |
| `description` | `string`   | ❌           | Descrição da responsabilidade do serviço | `"Servico responsavel pelo processamento de pedidos"` |
| `version`     | `string`   | ✅           | Versão do serviço                        | `"1.0.0"`                                             |
| `tags`        | `[]string` | ❌           | Tags para agrupamento e filtragem        | `["order", "checkout"]`                               |

---

## `request`

Dados sobre a requisição recebida pelo serviço.

> **Objeto opcional.** Preenchido via `collection.SetRequest(...)` ou `collection.SetRequestFromHTTP(...)`. Quando nenhum dos dois é chamado, o objeto inteiro é omitido do JSON — é assim que se registram logs de inicialização, jobs e workers. Os campos marcados como obrigatórios abaixo só valem **quando** o objeto está presente.

| Campo           | Tipo     | Obrigatório | Descrição                                                                        | Exemplo                 |
| --------------- | -------- | ----------- | -------------------------------------------------------------------------------- | ----------------------- |
| `communication` | `enum`   | ✅           | Protocolo de comunicação utilizado. Valores: `REST`, `gRPC`, `SOAP`, `WebSocket` | `"REST"`                |
| `method`        | `string` | ❌           | Método HTTP ou equivalente no protocolo                                          | `"POST"`                |
| `route`         | `string` | ❌           | Rota ou endpoint acessado                                                        | `"/v1/orders/checkout"` |
| `status`        | `int`    | ✅           | Código de status da resposta final                                               | `402`                   |
| `user_agent`    | `string` | ❌           | User-Agent do cliente                                                            | `"Mozilla/5.0"`         |

---

## `actor`

Contexto de quem disparou a requisição (usuário humano ou serviço).

> **Objeto opcional.** Preenchido via `collection.SetActor(...)`. Omitido do JSON quando não há um chamador identificável — o caso típico de traces de inicialização.

| Campo | Tipo | Obrigatório | Descrição | Exemplo |
|---|---|---|---|---|
| `id` | `string` | ✅ | Identificador único do actor | `"usr_9f3a1c"`, `"svc_payment"` |
| `type` | `enum` | ✅ | Tipo do actor. Valores: `USER`, `SERVICE` | `"USER"` |
| `ip` | `string` | ❌ | Endereço IP de origem da requisição | `"192.168.1.42"` |

---

## `time`

Dados de temporização da requisição completa.

| Campo | Tipo | Obrigatório | Descrição | Exemplo |
|---|---|---|---|---|
| `duration_ms` | `int` | ✅ | Duração total da requisição em milissegundos | `695` |
| `created_at` | `string (ISO 8601)` | ✅ | Timestamp de início da requisição | `"2026-05-24T10:32:15.123Z"` |
| `finished_at` | `string (ISO 8601)` | ✅ | Timestamp de fim da requisição | `"2026-05-24T10:32:15.818Z"` |

---

## `events[]`

Array de eventos rastreados durante o ciclo de vida da requisição. Quando `oltgo.StartEvent(ctx, ...)` é utilizado, os eventos são automaticamente aninhados em uma estrutura hierárquica (arvore) baseada no `parent_event_id`, com cada evento podendo conter subeventos no campo `events`.

| Campo | Tipo | Obrigatório | Descrição | Exemplo |
|---|---|---|---|---|
| `event_id` | `string` | ✅ | Identificador único do evento dentro do trace | `"evt_001"` |
| `parent_event_id` | `string \| null` | ❌ | ID do evento pai. Omitido para eventos raiz. Preenchido automaticamente ao usar `oltgo.StartEvent(ctx, ...)` | `"evt_002"`, `null` |
| `name` | `string` | ✅ | Nome descritivo da operação rastreada | `"process_payment"` |
| `type` | `enum` | ✅ | Categoria da operação. Valores: `FUNCTION`, `DATABASE`, `EXTERNAL_SERVICE`, `INTERNAL_SERVICE` | `"EXTERNAL_SERVICE"` |
| `status` | `enum` | ✅ | Resultado do evento. Valores: `SUCCESS`, `FAILURE` | `"FAILURE"` |
| `severity` | `enum` | ✅ | Nível de severidade do log. Valores: `DEBUG`, `INFO`, `WARN`, `ERROR`, `FATAL` | `"ERROR"` |
| `timestamp` | `string (ISO 8601)` | ✅ | Momento em que o evento foi iniciado | `"2026-05-24T10:32:15.234Z"` |
| `duration_ms` | `int` | ✅ | Duração do evento em milissegundos | `541` |
| `message` | `string` | ✅ | Mensagem legível descrevendo o resultado do evento | `"Falha ao processar pagamento"` |
| `metadata` | `map[string]any` | ❌ | Dados contextuais livres, específicos por tipo de evento | ver abaixo |
| `errors` | `[]EventError` | ❌ | Lista de erros ocorridos. Omitido quando vazio (`omitempty`) | ver abaixo |
| `events` | `[]Event` | ❌ | Subeventos aninhados (filhos deste evento). Montados automaticamente no `Commit()` a partir do `parent_event_id`. Omitido quando vazio (`omitempty`) | ver exemplo acima |

---

## `events[].metadata`

Campo livre (`map[string]any`). O conteúdo varia por `type` de evento. Exemplos por categoria:

### `type: FUNCTION`
| Campo sugerido | Tipo | Descrição |
|---|---|---|
| `method` | `string` | Método utilizado (ex: `"JWT"`) |
| `user_id` | `string` | ID do usuário processado |

### `type: DATABASE`
| Campo sugerido | Tipo | Descrição |
|---|---|---|
| `table` | `string` | Tabela acessada |
| `query` | `string` | Query executada |
| `rows_returned` | `int` | Número de linhas retornadas |

### `type: EXTERNAL_SERVICE`
| Campo sugerido | Tipo | Descrição |
|---|---|---|
| `provider` | `string` | Nome do provedor externo |
| `endpoint` | `string` | URL do endpoint chamado |
| `http_status` | `int` | Status HTTP retornado |
| `attempt` | `int` | Número da tentativa atual |
| `max_attempts` | `int` | Número máximo de tentativas |

### `type: INTERNAL_SERVICE`
| Campo sugerido | Tipo | Descrição |
|---|---|---|
| `service` | `string` | Nome do serviço interno chamado |
| `topic` | `string` | Tópico ou evento publicado/consumido |
| `correlation_id` | `string` | ID de correlação para rastreamento cross-service |

---

## `events[].errors[]`

Lista de erros associados a um evento. Omitida quando vazia.

| Campo | Tipo | Obrigatório | Descrição | Exemplo |
|---|---|---|---|---|
| `code` | `string` | ✅ | Código de erro padronizado | `"CARD_DECLINED"` |
| `message` | `string` | ✅ | Mensagem legível do erro | `"Cartão recusado pela operadora"` |
| `stack` | `string \| null` | ❌ | Stack trace. `null` quando não disponível. Omitido com `omitempty` | `null` |