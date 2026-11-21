---
id: 1
type: epic
title: "O Eduardo vê o estado das chaves numa página"
parent: initiative-multi-provider
covers: [R, 1, 2]
sources: []
companions: []
after: []
refined: true
risk: low
---

# O Eduardo vê o estado das chaves numa página

## Description

Uma página HTML renderizada no servidor mostra o estado, o cooldown e a porcentagem de uso de cada chave, a partir do mesmo relatório que o `/admin/keys` já monta. Não há API nova: o JSON do admin continua protegido. É a primeira entrega da iniciativa e não depende do roteamento por tiers.

## Outcome

Para o Eduardo: saber como está o roteador abrindo um endereço no navegador, sem curl nem Claude Code; o sinal é a página mostrar cada chave com estado, cooldown e uso.

## Requirements

- R1 (parent R12): `GET /status` devolve uma página HTML renderizada no servidor com, para cada chave, nome (ou posição, se não tiver nome), estado, cooldown e uso (`rolling`, `weekly`, `monthly`), sem exigir autenticação. Verificação: abrir `/status` no navegador contra o proxy rodando.
- R2 (parent R12): A página não contém o valor de nenhuma chave e nenhum endpoint JSON novo é criado; `GET /admin/keys` sem `PROXY_API_KEY` continua devolvendo 401. Verificação: teste que busca `/status` e `/admin/keys` sem chave.
- R3 (parent R12): Um erro na consulta de uso de uma chave aparece na linha dela e não derruba a página. Verificação: teste com o upstream de uso falhando para uma chave.
- R4 (parent R12): A página funciona no binário e na imagem Docker sem arquivo externo. Verificação: subir a imagem e abrir `/status`.

## Constraints

- Sem JSON novo e sem autenticação na página; `/admin/*` mantém a exigência de `PROXY_API_KEY` (iniciativa, Notes, 2026-10-29).
- As consultas de uso ao upstream reutilizam o cache existente (`usageCache`); a página aberta não cria consulta nova por visita.

## Done when

- `/status` mostra, para cada chave, nome, estado, cooldown e uso, renderizado no servidor.
- `/status` funciona sem autenticação e nunca expõe o valor de uma chave; `/admin/keys` sem chave devolve 401.
- A página vai no binário e na imagem Docker, e foi aberta contra o proxy rodando.
- `docs/operations.md` diz que a página é aberta e que, com URL público, a porta deve ser restrita por rede.

## Boundaries

- Fora: seção de tiers (épico roteamento-por-tiers) e relatório (épico relatorio-de-uso-e-custo).
- Touch point: o relatório de `handleKeysReport` em `adminkeys.go` (extraído para servir também a página) e `ServeHTTP` em `main.go` — owner: este épico
- Must not change: tudo sob `/admin/*` segue exigindo a `PROXY_API_KEY`; o JSON de `/admin/keys` não muda.

## References

- parent — _bmad-output/initiative-multi-provider/initiative-multi-provider.md, section Requirements
- decision — initiative-multi-provider.md, Notes, página sem autenticação (2026-10-29)
- code — `adminkeys.go`, `main.go` (`handleAdmin`)

## Notes

- Decision (2026-10-29): página renderizada no servidor, sem API exposta (sugestão do Eduardo); `/admin/keys` não é aberto.
- Assumption: o cache de uso existente basta para limitar as consultas ao upstream; sem limitação de taxa.
- Decision (2026-10-29): com `UsageCacheTTL=0` cada visita a `/status` consulta o upstream de uso; o Eduardo aceitou o risco (uso interno), sem piso de TTL.
- Decision (2026-10-29): a story 1 (`/status` com estado e uso das chaves) é o tracer bullet: o caminho mais fino por rota, relatório, template e teste; a story 2 (imagem Docker e documentação) vem depois.
