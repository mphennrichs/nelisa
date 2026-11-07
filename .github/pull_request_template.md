Closes #

## O que muda

<!-- Uma ou duas frases. Por que, se não for óbvio pela issue. -->

## Checklist

- [ ] Li `AGENTS.md` / `docs/arquitetura.md` e o PR segue as camadas
- [ ] Regra de negócio só em `api/`; o `web/` só consome a API
- [ ] Testes do comportamento novo (e da tentativa de violar regras, se tocar
      em quatro olhos, visibilidade ou permissões)
- [ ] Testes, lint e build rodados localmente e limpos
- [ ] `openapi.yaml` e cliente TS atualizados (se mudou endpoint)
- [ ] `CONTEXT.md` / ADR atualizados (se mudou termo ou decisão)
- [ ] Nenhum segredo, domínio fixo ou `runs-on: self-hosted`
- [ ] Usei agente de IA: [ ] sim [ ] não — e revisei o código mesmo assim
