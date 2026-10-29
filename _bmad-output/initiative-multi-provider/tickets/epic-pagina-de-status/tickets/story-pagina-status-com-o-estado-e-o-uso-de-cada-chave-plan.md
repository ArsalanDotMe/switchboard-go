---
title: 'Página /status com o estado e o uso de cada chave'
type: 'feature'
ticket: '1'
created: '2026-10-29'
status: done
baseline_revision: '271272114ce07e5aac86bc213295618d47871a93'
route: 'oneshot'
route_source: 'auto'
risk: 'low'
review: 'quick'
review_source: 'pinned'
lenses_ran: [quick]
review_loop_iteration: 0
followup_review_recommended: false
context: []
warnings: []
deferred:
  - summary: >-
      docs/operations.md, README.md e docs/admin-api.md não citam /status como rota aberta.
    evidence: |-
      operations.md diz que só /healthz e /readyz são abertas. A story 2 do épico cobre a documentação.
    location: >-
      docs/operations.md:60
    severity: low
  - summary: >-
      Cancelamento do contexto da requisição durante usageFor grava um erro de uso no cache pelo TTL.
    evidence: |-
      fetchUsage devolve o tipo do erro de contexto e usageFor o guarda sem checar ctx.Err(). Já era possível em /admin/keys; agora é alcançável sem autenticação.
    location: >-
      adminkeys.go:usageFor
    severity: medium
---

<intent-contract>

## Intent

**Problem:** O Eduardo só vê o estado e o uso das chaves via `GET /admin/keys` (JSON, exige `PROXY_API_KEY`), sem uma página que abra no navegador.

**Approach:** Extrair a montagem do relatório de `handleKeysReport` (`adminkeys.go`) para uma função `buildKeysReport(ctx)` reutilizada pelo handler JSON e por um novo `GET /status` em `ServeHTTP` (`main.go`), fora de `/admin/`, sem autenticação, renderizado com `html/template` embutido no código. Linha por chave: nome (ou posição + 1 se vazio), estado, cooldown (`RetryAfterSeconds`), `rolling`/`weekly`/`monthly` em % (traço quando ausente) e `UsageError` na linha da chave. Sem JSON novo; `/admin/keys` não muda.

</intent-contract>

## Implementation Notes

Oneshot: mudança pequena (~80 linhas em 2 arquivos + testes), um template e uma rota; sem decisão de design aberta. `UsageCacheTTL` e `usageCache` são reutilizados como estão. `html/template` escapa o nome e o erro. Só `GET` casa em `/status`.

## Verification

**Commands:**
- `go test ./...` -- expected: passa, com testes novos: `/status` sem chave devolve 200 e HTML; `/admin/keys` sem chave devolve 401; corpo de `/status` não contém o valor de nenhuma chave; upstream de uso falhando mostra o erro na linha da chave (a outra chave continua com o uso).

**Manual checks (if no CLI):**
- Com o proxy rodando, abrir `/status` e ver cada chave com estado, cooldown e uso.

## Review Triage Log

### 2026-10-29 — Review pass
- verdicts: 6 findings — high 0, medium 1, low 4, false 0, maybe-false 0 (1 rejected as accepted risk, counted low)
- findings:
  - `[low]` `[defer]` docs não citam /status como rota aberta — a story 2 do épico documenta a página.
  - `[low]` `[reject]` TTL=0 faz cada visita anônima consultar o upstream — o épico (Notes, 2026-10-29) registra que o Eduardo aceitou o risco, sem piso de TTL.
  - `[medium]` `[defer]` cancelamento de contexto envenena o cache de uso — já existia em /admin/keys, não é causado por esta mudança.
  - `[low]` `[patch]` comentário de UsageCacheTTL/UsageURL desatualizado — comentários em main.go atualizados.
  - `[low]` `[patch]` resposta sem Cache-Control — adicionado `Cache-Control: no-store`.
  - `[low]` `[reject]` lacunas de teste (POST /status, nome vazio no teste de erro) — não é defeito do código; o 401 duplicado é exigido pelo verify da story.

## Auto Run Result

- Resumo: `GET /status` serve uma página HTML renderizada no servidor, sem autenticação, com nome (ou posição), estado, cooldown e uso de cada chave; erro de uso aparece na linha da chave.
- Arquivos: `adminkeys.go` (`buildKeysReport` extraída, template e `handleStatusPage`), `main.go` (rota `/status`, comentários), `main_test.go` (2 testes novos).
- Revisão: 2 patches (low), 2 adiados, 2 rejeitados com motivo no log.
- Follow-up review recomendado: false (nenhum patch high, menos de 2 medium).
- Verificação: `gofmt`, `go vet` e `go test -count=1 ./...` passam. Abrir `/status` contra o proxy rodando não foi feito.
- Riscos residuais: o cache de uso pode guardar erro de cancelamento (adiado); com `USAGE_CACHE_TTL=0` cada visita consulta o upstream (aceito).

## Code Review

### 2026-10-29
Code review complete. 0 `decision-needed`, 7 `patch`, 0 `defer`, 14 rejected.

- [x] [Review][Patch] Cancelar a requisição durante usageFor grava erro no cache de uso (agora alcançável sem autenticação); não guardar entradas quando ctx.Err() != nil [adminkeys.go:usageFor]
- [x] [Review][Patch] Testes de /status não verificam a célula de cooldown nem o marcador "(atual)" [main_test.go:TestStatusPageOpenAndNoKeyLeak]
- [x] [Review][Patch] Janela de uso parcial (traço "—") não é exercitada; um 500 por nil não seria pego [main_test.go]
- [x] [Review][Patch] `Cache-Control: no-store` não é verificado [main_test.go]
- [x] [Review][Patch] Erro de uso não é fixado na linha da chave, escape de HTML (KeyNames/UsageError) não é testado e as asserções por substring são frouxas ("7.0%" casa "17.0%") [main_test.go:TestStatusPageUsageErrorOnKeyRow]
- [x] [Review][Patch] HEAD /status devolve 404; aceitar HEAD [main.go:623]
- [x] [Review][Patch] docs/operations.md diz que só /healthz e /readyz são abertas; incluir /status [docs/operations.md:60]

Rejected:
- Gastar cota com visitas anônimas: falso, usageFor segura o mutex no refresh e coalesce; limite é chaves por TTL (TTL=0 aceito no épico).
- Nil em .Usage no template: falso, fetchUsage devolve usage não-nil quando err vazio e usageFor preenche todo índice.
- UsageError vaza detalhe do upstream: falso, só "HTTP n", timeout ou nome de tipo Go.
- Contagem do log de revisão / check manual não feito: falso, contas batem e o check pendente está declarado.
- usage[i] fora do limite: falso, KeyManager e usage nascem da mesma lista imutável.
- Mutex de usageFor ignora ctx: baixo, espera limitada a 5s, correção exige lock com ctx.
- Zero chaves / ProxyAPIKey vazia sem teste: baixo, zero chaves é rejeitado na config (main.go:319).
