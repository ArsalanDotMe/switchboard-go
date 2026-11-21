---
title: 'Verificar /status na imagem Docker e documentar'
type: 'chore'
ticket: '2'
created: '2026-10-29'
status: done
baseline_revision: 'a47026df02148b60bb9207e3a59065eb63c35348'
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
deferred: []
---

<intent-contract>

## Intent

**Problem:** A página `/status` (story 1) nunca foi aberta na imagem Docker, e o README e `docs/operations.md` não dizem claramente que ela é aberta nem que, com URL público, a porta deve ser restrita por rede.

**Approach:** Construir a imagem com o `Dockerfile` existente, subir com chaves falsas, abrir `/status` e confirmar as chaves na página sem arquivo externo (template embutido no binário). Em `README.md`, listar `GET /status` entre as rotas abertas; em `docs/operations.md`, acrescentar o aviso de restringir a porta por rede quando o URL for público (a lista de rotas abertas já cita `/status`).

</intent-contract>

## Implementation Notes

Imagem construída e executada com chaves falsas: `/status` devolveu 200 `text/html`, listou `#1 (atual)` e `#2` com erro de uso `HTTP 401` (esperado com chaves falsas), sem nenhum valor de chave no corpo; `/admin/keys` sem chave devolveu 401. Arquivos: `README.md` (rota `/status` na lista de rotas abertas, com aviso de rede), `docs/operations.md` (aviso de restringir a porta por rede com URL público). Imagem de teste removida.

Oneshot: verificação manual da imagem mais duas edições curtas de documentação, sem código Go. `docs/admin-api.md` fica fora (só `/admin/*` e health). Se a imagem não subir ou `/status` falhar, o defeito é de código e a story pára com a evidência.

## Verification

**Commands:**
- `docker build -t switchboard-go:status-check .` -- expected: build conclui
- `docker run` com `PROXY_API_KEY` e `OPENCODE_GO_API_KEYS` falsos e `LISTEN_ADDR=0.0.0.0:8080`, depois `curl -s localhost:<porta>/status` -- expected: 200, HTML com as chaves por nome ou posição e sem o valor delas
- `go test -count=1 ./...` -- expected: passa

**Manual checks (if no CLI):**
- `README.md` e `docs/operations.md` listam `/status` como rota aberta e trazem o aviso de rede.

## Review Triage Log

### 2026-10-29 — Review pass
- verdicts: 5 findings — high 0, medium 0, low 2, false 2, maybe-false 0 (1 low rejected)
- findings:
  - `[low]` `[reject]` `/status` fica sob o título "Admin endpoints" do README — a lista de health checks já vivia ali e o item está numa sublista separada "Open routes"; renomear o título não vale a mudança.
  - `[low]` `[patch]` aviso de rede não explica o risco real (visita anônima dispara consulta de uso com as chaves) — frase acrescentada em `docs/operations.md`.
  - `[false]` `[reject]` `go test` sem registro — rodou no Verify e passou (`ok switchboard-go`); consta em Auto Run Result.
  - `[false]` `[reject]` diff com metadados de review antigos — o diff foi gerado antes de gravar os campos; o que vai no commit é a árvore de trabalho.
  - `[low]` `[reject]` README não cita HEAD — inofensivo, sem correção útil.

## Auto Run Result

- Resumo: imagem Docker construída e executada com chaves falsas; `/status` abriu sem arquivo externo. README e `docs/operations.md` documentam a rota aberta e o aviso de restringir a porta por rede.
- Arquivos: `README.md` (`GET /status` entre as rotas abertas), `docs/operations.md` (aviso de rede e de consulta de uso por visita).
- Revisão: 1 patch (low), 0 adiados, 4 rejeitados com motivo no log.
- Follow-up review recomendado: false.
- Verificação: `docker build` ok; container respondeu 200 `text/html` em `/status` com `#1 (atual)` e `#2` (erro de uso `HTTP 401`, esperado com chaves falsas), sem valor de chave no corpo; `/admin/keys` sem chave deu 401; `go test -count=1 ./...` passa.
- Riscos residuais: nenhum novo; uso real contra o OpenCode Go não testado (chaves falsas).

## Code Review

### 2026-10-29

Code review: 0 decision-needed, 0 patch, 0 defer, 13 rejected. Lenses: blind-hunter, edge-case-hunter, verification-gap, intent-alignment.

#### Rejected
- Blind: README omits usage-query note — low, operations.md carries it; README already says to restrict the port.
- Blind: no mitigation for anonymous upstream trigger — false: `usageFor` holds a mutex (visits coalesce) and caches per key for `UsageCacheTTL` (60s, `USAGE_CACHE_TTL`).
- Blind: key-leak path with key-bearing upstream error untested — false for this diff: docs-only, handler reviewed in 1.1; `fetchUsage` keeps only HTTP status or error type.
- Blind: Docker check not repeatable — low, a CI smoke step is extra scope.
- Blind: open-routes sublist under the Admin heading — low, heading rename already rejected in the plan.
- Blind: `docs/admin-api.md` lacks `/status` — low, outside the ticket's named files.
- Blind: `/status` format undocumented — low, README describes the content in one line.
- Blind: stale ticket metadata — low, workflow artifact, not product.
- Blind: long line in operations.md — low, cosmetic.
- Intent: Docker verification attested only in plan prose — descriptive, not a defect; the Verify step is an action recorded in the plan.
- Intent: operations.md already listed `/status` — requirement met, only the warning was missing.
- Edge Case Hunter and Verification Gap: no findings.
