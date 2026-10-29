# 0005 — CI só em runners do GitHub; fogolab puxa as imagens

**Status:** aceita (2026-10-28)

## Contexto
Os outros projetos usam o runner self-hosted `fogoserver`. O Nelisa tem repo
**público**: um PR de terceiro com workflow alterado executaria código
arbitrário na máquina de casa. Repo público tem Actions grátis nos runners do
GitHub.

## Decisão
- CI (lint, testes, build) só em runners hospedados pelo GitHub.
- Imagens publicadas no GHCR apenas em push na `main`, nunca a partir de PR.
- O fogolab **puxa** a imagem para o deploy; o GitHub nunca entra na rede local.

## Consequências
- Nenhum job do Nelisa pode usar `runs-on: self-hosted`.
- Mesmo pipeline serve para a AWS depois: muda só o destino.
