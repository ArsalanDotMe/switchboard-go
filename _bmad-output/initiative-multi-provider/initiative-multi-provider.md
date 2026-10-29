---
type: initiative
title: "O switchboard-go atende os agentes por vários provedores, com limite de gasto, e mostra o uso"
sources:
  - _bmad-output/forge-multi-provider/forge-multi-provider.md
companions: []
after: []
risk: medium
---

# O switchboard-go atende os agentes por vários provedores, com limite de gasto, e mostra o uso

## Description

Hoje o proxy fala com um upstream (OpenCode Go) e gira um pool de chaves. Quando a cota acaba, os agentes Hermes ficam sem resposta. Ao fim da iniciativa, o proxy tem tiers de provedores em ordem de prioridade (OpenCode Go primeiro, depois outros e gratuitos), troca de tier sozinho quando um falha, respeita um teto diário nos pagos e mostra o uso em uma página e em um relatório. Os agentes continuam com o mesmo URL e a mesma `PROXY_API_KEY`. Fontes: o forge `multi-provider` e a sessão de planejamento de 2026-10-29.

## Outcome

Para o Eduardo e seus agentes Hermes: uma request nunca falha por cota esgotada enquanto algum tier elegível responder, sem mudar nenhum agente; o sinal é um teste com OpenCode Go esgotado em que o próximo tier mapeado atende.

## Requirements

- R1 (forge, Decisões): O proxy escolhe tiers por prioridade, OpenCode Go = 0, depois os demais na ordem da config; se um tier falha, o próximo atende a request.
- R2 (forge, Decisões): Cada tier tem um mapa de modelos; um modelo sem mapeamento faz o tier ser pulado.
- R3 (forge, Decisões): Cada tier declara os estilos de API que fala (OpenAI, Anthropic) e é pulado se não falar o do request; não há tradução entre estilos.
- R4 (forge, Decisões): Toda a config fica no proxy; os agentes mantêm o mesmo URL e a mesma `PROXY_API_KEY`.
- R5 (forge, Decisões): Todo tier pago tem teto diário global, compartilhado entre agentes; uma config sem ele é rejeitada na partida; o primeiro uso de um tier pago dispara o alerta SMTP.
- R6 (forge, Decisões): Tiers gratuitos (`free: true`) entram como os demais.
- R7 (forge, Decisões): 429, 5xx, 401, 402 e 404 do upstream colocam o tier em cooldown e a request segue ao próximo; 400 não faz failover.
- R8 (forge, Decisões): Um modelo só é elegível num tier que tenha o contexto mínimo e o tool calling que o Hermes precisa.
- R9 (forge, Decisões): A automação de catálogo (modelos novos, limites novos, quem deixou de ser gratuito) só propõe e nunca altera o roteamento.
- R10 (sessão de 2026-10-29): O estado de cooldown e o gasto do dia sobrevivem a um reinício do proxy.
- R11 (sessão de 2026-10-29): Cada request gera um log estruturado com modelo, estilo, chave ou tier, status e latência.
- R12 (sessão de 2026-10-29): Uma página de status HTML mostra chaves (depois tiers), estado, cooldown e uso; é a primeira entrega.
- R13 (sessão de 2026-10-29): Um relatório de uso e custo agrega por dia, projeto, modelo, chave e tier; o custo vem de uma tabela de preços na config, e sem preço o relatório mostra só tokens.
- R14 (sessão de 2026-10-29): O projeto é identificado pelo cabeçalho opcional `X-Project`; sem ele a request fica "sem projeto"; o proxy não repassa o cabeçalho ao upstream e limita tamanho e caracteres do valor.
- R15 (sessão de 2026-10-29): A config é recarregada por SIGHUP sem derrubar requests em andamento.

## Constraints

- Sem tradução entre estilos de API: caro (SSE, tools) e desnecessário para o Hermes (forge, Rejeitado).
- Sem proxy key por agente nem por projeto; adiado, não descartado (forge; R14).
- Privacidade aceita: nenhum agente lida com dado sensível, então tiers gratuitos recebem o contexto como os demais (forge, Decisões).
- O catálogo automático não altera o roteamento; a auto-adição começa pelo OpenRouter, só depois de ver a qualidade das propostas (forge, Decisões).
- Preço legível por máquina só no OpenRouter; nos demais a tabela é preenchida à mão (forge, Decisões; R13).

## Done when

- Com a cota do OpenCode Go esgotada, uma request é atendida pelo próximo tier com modelo mapeado, sem alteração nos agentes.
- Uma config com tier pago sem teto diário é rejeitada na partida.
- Cooldown e gasto do dia permanecem depois de reiniciar o proxy.
- A página de status mostra chaves e tiers, estado, cooldown e uso do dia.
- O relatório mostra uso por dia, projeto, modelo, chave e tier, com custo onde há preço.
- O catálogo produz propostas de modelos e limites sem tocar no roteamento.

## Boundaries

- Fora: tradução entre estilos, proxy key por agente ou projeto, auto-adição de catálogo, tratamento de dado sensível.
- Touch point: `KeyManager` em `main.go` — vira o gerenciador de tiers; owner: epic-roteamento-por-tiers
- Touch point: `/admin/keys` em `adminkeys.go` — alimenta a página de status; owner: epic-pagina-de-status
- Must not change: o URL e a `PROXY_API_KEY` que os agentes Hermes usam hoje.
- Caminho completo: uma request do Hermes chega, o OpenCode Go devolve 429, o proxy mapeia o modelo no próximo tier, atende, grava o log e o uso aparece na página.

## References

- forge — `_bmad-output/forge-multi-provider/forge-multi-provider.md`, Decisões, Rejeitado, A verificar na construção
- code — `main.go` (`KeyManager`, `proxyV1`), `adminkeys.go` (`handleKeysReport`)

## Notes

- Decision (2026-10-29): o estado e o gasto do dia persistem em arquivo JSON local escrito de forma atômica, sem banco.
- Open question: qual o estilo real do Hermes, OpenAI ou `/v1/messages`? Ver nos logs do proxy (MiniMax pode usar `/v1/messages`). Espera: o épico de roteamento.
- Open question: o cooldown dos tiers gratuitos, já que o 429 deles é por minuto ou por dia. Espera: o épico de roteamento.
- Open question: de onde vem preço e gratuidade fora do OpenRouter, que não tem API. Espera: o épico de catálogo.
- Open question: o Hermes consegue mandar o cabeçalho `X-Project`? Sem isso o relatório por projeto fica vazio. Espera: o épico de relatório.
- Decision (2026-10-29): iniciativa com épicos separados: página de status primeiro, depois roteamento por tiers, relatório de uso e custo, e catálogo automático (este só registrado, sem fatiar).
- Decision (2026-10-29): R10 a R15 nasceram na sessão de planejamento de 2026-10-29 (registrada no `.memlog.md`), não no forge.
- Decision (2026-10-29): sem épico de base de plataforma; o projeto já tem binário, CI (`.goreleaser.yaml`, `.github/`) e Docker, e o primeiro épico é um recurso.
- Decision (2026-10-29): a primeira story do roteamento separa `main.go` em arquivos dentro do `package main`, sem pacotes nem interfaces novas.
- Parked: R15 (recarga por SIGHUP). Ticket avulso em `backlog/`, por decisão do Eduardo; só depois do épico de roteamento.
- Source conflict: R7 — o código só troca de chave em 429 de cota (`isQuota429` em `main.go`); 401, 402, 404 e 5xx passam ao cliente e erro de conexão vira 502 sem failover. R7 muda esse comportamento; o épico de roteamento o entrega.
- Open question: erro de conexão e timeout do upstream também fazem failover (R7 lista só status HTTP)? Espera: o épico de roteamento.
- Open question: as várias chaves do OpenCode Go continuam girando dentro do tier 0 antes de passar ao tier seguinte? Espera: o épico de roteamento.
- Decision (2026-10-29): a página de status é renderizada no servidor e fica sem autenticação; uso interno, dados não críticos. Não há API nova e `/admin/*` segue protegido. Nunca mostra o valor das chaves, só nome, estado, cooldown e uso.
