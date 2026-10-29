# 0004 — Next.js no front (SSR na parte pública), Go só como API

**Status:** aceita (2026-10-28)

## Contexto
A stack pedida é React + Go. A parte pública precisa de prévia de link
(WhatsApp e redes não executam JS) e de SEO ("PL 1234/2026" no Google). Uma SPA
pura não entrega nenhum dos dois.

## Decisão
`web/` em Next.js: parte pública renderizada no servidor com cache; área
restrita como app no cliente. `api/` em Go é a única dona dos dados e das
regras; o Next só consome a API.

## Alternativas descartadas
- SPA + Go injetando meta tags: resolve prévia, SEO fraco, gambiarra.
- Parte pública em templates Go + SPA só na área restrita: um runtime só, mas
  o público deixa de ser React.

## Consequências
- Dois runtimes (Node e Go) para build, deploy e monitorar.
- Regra de negócio **nunca** no Next — senão vira dois backends.
