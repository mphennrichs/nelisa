# Nelisa

Projetos de lei federais, estaduais e municipais explicados: resumo, texto
comentado por dispositivo e tramitação, revisados por uma equipe com regra de
quatro olhos. Leitores contribuem sem cadastro.

- Glossário do domínio: [`CONTEXT.md`](CONTEXT.md)
- Decisões de arquitetura: [`docs/adr/`](docs/adr/)
- Nomes no código: [`docs/nomes-no-codigo.md`](docs/nomes-no-codigo.md)
- Arquitetura e padrões: [`docs/arquitetura.md`](docs/arquitetura.md)
- Como contribuir: [`CONTRIBUTING.md`](CONTRIBUTING.md) · agentes de IA: [`AGENTS.md`](AGENTS.md)
- Plano até o deploy da v1: [`docs/plano.md`](docs/plano.md)
- Andamento: [quadro Nelisa v1](https://github.com/users/mphennrichs/projects/1) e [issues](https://github.com/mphennrichs/nelisa/issues)

## Próximos passos

Fase 0 do [plano](docs/plano.md): API em Go com Postgres e login real no lugar
do protótipo. O beta público (sem divulgação) abre no fogolab ao fim da fase 3;
a migração para a AWS vem depois.

## Estado atual

- `/` — página "em construção" com link para a demonstração.
- `/demo` — protótipo em memória (Next.js, sem banco), público, com contas de
  teste (senha `nelisa123`). Serve para apresentar o projeto.
- `/equipe` — área real da equipe, com login da API em Go (`api/`).

```sh
# API + Postgres de desenvolvimento
docker compose -f api/compose.dev.yaml up -d
cp api/.env.example api/.env   # e exporte as variáveis
(cd api && go run ./cmd/nelisa migrate && go run ./cmd/nelisa api)

# Web
cd web && npm install && API_INTERNAL_URL=http://localhost:8080 npm run dev
```

Na primeira vez, crie o Admin em http://localhost:3000/equipe/primeiro-acesso
com o valor de `BOOTSTRAP_TOKEN`.
