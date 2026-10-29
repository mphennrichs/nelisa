# 0005 — CI só em runners do GitHub; fogolab puxa as imagens

**Status:** aceita (2026-10-28)

## Contexto
Os outros projetos usam o runner self-hosted `fogoserver`. O Nelisa tem repo
**público**: um PR de terceiro com workflow alterado executaria código
arbitrário na máquina de casa. Repo público tem Actions grátis nos runners do
GitHub.

## Decisão
- CI (lint, testes, build) só em runners hospedados pelo GitHub.
- O deploy da v1 usa o **local-build do fogolab**, como os outros projetos:
  o host sincroniza a `main` do GitHub (via Forgejo), builda a imagem lá
  mesmo, publica em `localhost:5000/nelisa`, faz o bump do compose e o
  Komodo sobe. Só a `main` é buildada, e só o dono faz merge nela.
- O fogolab **puxa**; o GitHub nunca entra na rede local.

## Consequências
- Nenhum job do Nelisa pode usar `runs-on: self-hosted`.
- O runner `fogoserver` nunca é registrado neste repo.
- Na AWS, a imagem passa a ser publicada num registry; o resto do pipeline
  não muda.

_Atualização 2026-10-28:_ deploy via local-build do fogolab (antes: imagens
no GHCR).
