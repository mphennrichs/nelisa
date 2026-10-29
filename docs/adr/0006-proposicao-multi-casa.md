# 0006 — Proposição, não PL; uma Proposição atravessa várias Casas

**Status:** aceita (2026-10-28)

## Contexto
O glossário nasceu com "Projeto de Lei (PL)" como objeto central e cada PL
pertencendo a uma Casa. O primeiro exemplo real quebrou as duas ideias: a PEC
221/2019 (fim da escala 6x1) é uma PEC, não um PL; foi iniciada na Câmara,
revista no Senado e virou a EC 141/2026. A PEC 8/2025 foi apensada a ela e
declarada prejudicada.

## Decisão
- O objeto central é a **Proposição**, com um **Tipo** (PL, PEC, PLP, MPV...).
- Uma Proposição tramita por uma ou mais Casas e é **um registro só**, com
  Tramitação única; a primeira Casa é a iniciadora.
- Apensação é uma relação entre Proposições; Prejudicada é um desfecho.

## Consequências
- No código, `Bill` vira `Proposition` (ver `docs/nomes-no-codigo.md`).
- Importação precisa casar o registro da Câmara com o do Senado (os IDs são
  diferentes em cada API) — pendente de desenho.
- ADRs 0001–0005 ainda falam "PL"; leia como "Proposição".
