# Multi-provider no switchboard-go

Trabalho avulso, sem iniciativa. Mudança num projeto existente: hoje há um upstream (OpenCode Go) e um pool de chaves.

## Decisões

- Tiers de prioridade: OpenCode Go = 0, depois outros (OpenAI, OpenRouter, Z.ai, Anthropic, tiers gratuitos), na ordem da config.
- Cada tier tem um mapa de modelos (`glm-5.1` → equivalente). Sem mapeamento, o tier é pulado. Substituição aceita: só o Eduardo usa.
- Sem tradução entre OpenAI-style e Anthropic-style. Cada tier declara os estilos que fala e é pulado se não falar o do request.
- Toda a config fica no proxy. Os agentes Hermes continuam com o mesmo URL e a mesma `PROXY_API_KEY`.
- Tier pago exige teto diário global, compartilhado entre agentes. Config sem ele é inválida. Primeiro uso de tier pago dispara o alerta SMTP.
- Tiers gratuitos (`free: true`) entram como os demais. Sem proxy key por agente: nenhum agente lida com dado sensível.
- Falha do upstream (429, 5xx, 401, 402, 404) conta como rate limit: cooldown e próximo tier. 400 não faz failover.
- Um modelo só é elegível num tier se tiver o mínimo de contexto e de tool calling que o Hermes precisa.
- Automação de catálogo (modelos novos, limites, quem deixou de ser gratuito) só propõe. Auto-adição fica para depois, começando pelo OpenRouter (preço via API).

## Rejeitado

- Cadeia de fallback escolhida por request: obrigaria configurar cada agente.
- Fallback só com o mesmo modelo: deixaria OpenAI e Anthropic quase sempre de fora.
- Tradução entre estilos de API: caro (SSE, tools) e desnecessário para o Hermes.
- Proxy key por agente: adiada, sem necessidade hoje.

## A verificar na construção

- Estilo real do Hermes (OpenAI ou Anthropic) nos logs do proxy; modelos como MiniMax podem usar `/v1/messages`.
- Cooldown adequado para tiers gratuitos (o 429 deles é por minuto ou por dia).
- Fonte de preço e gratuidade fora do OpenRouter: sem API, só página de documentação.
