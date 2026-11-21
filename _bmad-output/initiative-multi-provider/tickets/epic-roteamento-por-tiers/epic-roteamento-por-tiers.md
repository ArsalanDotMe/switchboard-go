---
id: 2
type: epic
title: "O proxy troca de provedor quando um falha"
parent: initiative-multi-provider
covers: [R, 1, ,,  , R, 2, ,,  , R, 3, ,,  , R, 4, ,,  , R, 5, ,,  , R, 6, ,,  , R, 7, ,,  , R, 8, ,,  , R, 1, 0, ,,  , R, 1, 1]
sources: []
companions: []
after: []
refined: false
risk: high
---

# O proxy troca de provedor quando um falha

## Description

O proxy atende a request pelo primeiro tier elegível, em ordem de prioridade, e passa ao próximo quando um falha. Cada tier tem mapa de modelos, estilos de API, flag de gratuito e, se pago, teto diário global com alerta SMTP. O estado e o gasto do dia sobrevivem a reinício e cada request gera um log estruturado. Entrega ao relatório e ao catálogo o log, o estado persistido, a lista de tiers e o formato da config; entrega à página a seção de tiers.

## Outcome

Para o Eduardo e os agentes Hermes: uma request não falha por cota esgotada enquanto algum tier elegível responder, sem mudar nenhum agente; o sinal é um teste com o OpenCode Go esgotado em que o próximo tier mapeado atende.

## Done when

- Com o OpenCode Go esgotado, uma request é atendida pelo próximo tier com modelo mapeado, verificado contra upstreams de teste.
- 401, 402, 404, 5xx e 429 põem o tier em cooldown e passam ao próximo; 400 volta ao cliente.
- Um tier sem mapeamento para o modelo, ou que não fala o estilo do request, é pulado.
- Uma config com tier pago sem teto diário é rejeitada na partida; o primeiro uso de tier pago dispara o alerta SMTP.
- Cooldown e gasto do dia permanecem depois de reiniciar o proxy.
- Cada request grava uma linha de log com modelo, estilo, tier, status e latência, e a página de status mostra os tiers.

## Boundaries

- Fora: tradução entre estilos, proxy key por agente, relatório e catálogo.
- Touch point: `KeyManager` e `proxyV1` em `main.go` — owner: este épico
- Touch point: a página de status — owner: epic-pagina-de-status; este épico acrescenta a seção de tiers
- Must not change: o URL e a `PROXY_API_KEY` que os agentes Hermes usam.

## References

- parent — _bmad-output/initiative-multi-provider/initiative-multi-provider.md, section Requirements
- forge — _bmad-output/forge-multi-provider/forge-multi-provider.md, Decisões e A verificar na construção
- code — `main.go` (`KeyManager`, `proxyV1`, `isQuota429`)

## Notes

- Decision (2026-10-29): o estado e o gasto do dia persistem em arquivo JSON local escrito de forma atômica, sem banco.
- Decision (2026-10-29): a primeira story separa `main.go` em arquivos dentro do `package main`, sem pacotes nem interfaces novas.
- Source conflict: R7 — o código só troca de chave em 429 de cota (`isQuota429`); 401, 402, 404 e 5xx passam ao cliente e erro de conexão vira 502 sem failover. Este épico muda isso.
- Open question: qual o estilo real do Hermes, OpenAI ou `/v1/messages`? Ver nos logs do proxy.
- Open question: o cooldown dos tiers gratuitos, já que o 429 deles é por minuto ou por dia.
- Open question: erro de conexão e timeout do upstream também fazem failover?
- Open question: as várias chaves do OpenCode Go continuam girando dentro do tier 0?
