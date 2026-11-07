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

## Estado atual: protótipo

`web/` é uma prova de conceito em Next.js com **tudo em memória**: dados
fictícios, sem banco, sem chamadas externas, sem API em Go ainda. Serve para
validar telas e regras (visibilidade pública, fila, posse, quatro olhos,
contribuições).

```sh
cd web
npm install
npm run dev
```

Abra http://localhost:3000. A área da equipe fica em `/equipe`; as contas de
teste aparecem na tela de login (senha `nelisa123`).
