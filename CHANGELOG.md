# Changelog

## v1.1.0

### Correções de perda de dados
- `buildEventTree`: ciclos em `parent_event_id` não apagam mais todos os eventos
  do trace (antes: `events: null`). Eventos em ciclo são promovidos a raiz.
- `buildEventTree`: `event_id` duplicado não duplica um evento nem descarta outro.
  A primeira ocorrência mantém o id; as demais permanecem como nós independentes.
- `Commit()` é idempotente: chamadas repetidas não reenviam o trace.

### Coerência entre severity, status e errors  (mudança de comportamento)
- Eventos com severidade `ERROR` ou `FATAL` agora são marcados como
  `status: FAILURE`, e o trace que os contém como `FAILURE`. Antes eram
  reportados como `SUCCESS` e não apareciam em nenhum filtro de erro.
- `AddError` passa a elevar a severidade para no mínimo `ERROR` (sem rebaixar
  `FATAL`). Antes um evento com erro registrado podia permanecer em `INFO`.
- Novos métodos `EventBuilder.Fail()` e `EventBuilder.Succeed()`: `Fail()` marca
  falha sem exigir um objeto de erro; `Succeed()` mantém `SUCCESS` em um evento
  de severidade `ERROR`/`FATAL` — o caso do erro tratado com a execução seguindo
  por outro caminho. O override vale também no status do trace.
- `AddEvent` deriva o status da severidade apenas quando `Status` não é
  informado; um valor explícito é respeitado.
- `severity` `WARN` permanece compatível com `SUCCESS` — "retry teve sucesso" é
  um caso legítimo e não foi alterado.
- Novo helper exportado `IsFailureSeverity(Severity) bool`.

### Mudanças no formato do JSON
- Timestamps (`time.created_at`, `time.finished_at`, `events[].timestamp`) agora
  são serializados em UTC com sufixo `Z`, em vez do horário local com offset.
  O instante representado é o mesmo; parsers RFC 3339 não são afetados. A
  duração continua vindo do relógio monotônico — a conversão acontece só na
  serialização, depois do cálculo.
- `parent_event_id` é omitido em eventos raiz, em vez de emitido como `null`
  — alinhando o comportamento com a documentação.
- `event_id` gerado passa de 3 para 6 dígitos (`evt_000001`), afastando a
  quebra de ordenação lexicográfica a partir do milésimo evento. Ainda assim,
  `event_id` não deve ser usado para ordenação: use `timestamp`.

### Novidades
- `Shipping.Stats()` e `Agent.Stats()` expõem contadores de traces processados,
  falhos e descartados por buffer cheio ou por `Ship` após `Close()`.
- `NewAgentWithOptions` permite configurar o tamanho do buffer e um callback
  `OnError` para erros do `ProcessLog`. `NewAgent` permanece inalterado.
- Constante `Staging` adicionada a `Environment`.

### Documentação
- `docs/log.md`: `parent_event_id` documentado como omitido (agora verdadeiro);
  valores de `environment` corrigidos; nota de que `event_id` não deve ser usado
  para ordenação; regras de coerência entre `status`, `severity` e `errors`
  documentadas.
- `README.md`: novas seções sobre coerência de status/severidade e sobre a
  observabilidade do pipeline (`Stats`, `NewAgentWithOptions`).

### Notas de migração
A mudança mais visível é a de status: traces que hoje aparecem como `SUCCESS`
passam a `FAILURE` — o que é a correção, mas move números de dashboard e pode
disparar regras de alerta que estavam quietas por estarem medindo errado.

Um `grep` por `SeverityError` e `SeverityFatal` na sua base mostra exatamente
quais eventos mudam de status. Onde o uso for deliberado ("erro tratado, a
operação seguiu"), acrescentar `.Succeed()` preserva o comportamento antigo de
forma explícita.

## v1.0.0

Release inicial do MVP.
