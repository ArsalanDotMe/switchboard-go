---
id: 4
type: epic
title: "O Eduardo recebe propostas de modelos e limites novos"
parent: initiative-multi-provider
covers: [R, 9]
sources: []
companions: []
after: []
refined: false
risk: medium
---

# O Eduardo recebe propostas de modelos e limites novos

## Description

Uma automação recorrente pesquisa modelos novos, limites novos e quem deixou de ser gratuito, e grava propostas de mudança na config de tiers. Só propõe: nunca altera o roteamento. Auto-adição, começando pelo OpenRouter (preço via API), fica para depois de ver a qualidade das propostas.

## Outcome

Para o Eduardo: ser avisado de mudanças no catálogo antes de gastar sem querer; o sinal é uma proposta que aponta um modelo que passou de gratuito a pago.

## Done when

- Uma rodada da automação grava uma proposta com modelos novos, limites novos e mudanças de gratuidade.
- O roteamento é idêntico antes e depois de uma rodada.
- A automação roda sozinha em intervalo recorrente.

## Boundaries

- Fora: auto-adição de modelos e qualquer escrita na config de tiers.
- Touch point: o formato da config de tiers — owner: epic-roteamento-por-tiers; a tabela de preços — owner: epic-relatorio-de-uso-e-custo
- Must not change: a config de tiers em uso.

## References

- parent — _bmad-output/initiative-multi-provider/initiative-multi-provider.md, section Requirements
- forge — _bmad-output/forge-multi-provider/forge-multi-provider.md, Decisões (automação de catálogo)

## Notes

- Open question: de onde vem preço e gratuidade fora do OpenRouter, que não tem API? Só página de documentação.
- Decision (2026-10-29): este épico fica só registrado, sem stories, até o roteamento e o relatório existirem.
