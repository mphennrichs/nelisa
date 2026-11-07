# Instruções para agentes de código

Este arquivo vale para qualquer agente (Claude Code, Codex, Cursor, Copilot
etc.) que trabalhe neste repositório. Siga-o antes de qualquer preferência
própria.

## Antes de escrever código, leia

1. A issue que você está resolvendo. Se não houver issue, pare e peça uma.
2. [`CONTEXT.md`](CONTEXT.md): o vocabulário do domínio. Use os termos dele;
   não invente sinônimos.
3. [`docs/nomes-no-codigo.md`](docs/nomes-no-codigo.md): a tradução oficial
   dos termos para o código (em inglês).
4. [`docs/arquitetura.md`](docs/arquitetura.md): estrutura, camadas e
   padrões. É obrigatório.
5. Os ADRs em [`docs/adr/`](docs/adr/) que tocam a área da issue.

## Regras que não se quebram

- **Regra de negócio só em `api/`** (Go). O `web/` nunca decide visibilidade,
  permissão ou estado de revisão (ADR 0004).
- **Camadas:** `domain/` não importa infraestrutura; um módulo não importa
  `domain/`, `store/` ou `httpapi/` de outro, só fala por portas.
- **Quatro olhos:** quem escreve um conteúdo nunca o aprova. Não crie atalho,
  flag ou papel que contorne isso.
- **Visibilidade:** a camada pública só expõe conteúdo aprovado. Toda rota
  `/api/public/...` passa por ela.
- **Auditoria:** toda ação da equipe que muda conteúdo, permissão ou revisão
  grava evento na mesma transação.
- **Nada de URL arbitrária:** a v1 não busca páginas indicadas por usuários.
- **IA:** só rascunho de Resumo e Estruturação, revisado por humano; nunca
  Comentários (ADR 0003).
- **CI:** nunca `runs-on: self-hosted` nem `pull_request_target` com
  checkout do PR (ADR 0005).
- **Sem segredos** no código, nos testes ou nos logs.
- **Sem domínio fixo:** use `PUBLIC_BASE_URL`.
- **Escopo:** faça o que a issue pede. Refatoração, dependência nova ou
  mudança de arquitetura fora disso vira outra issue.

## Comandos

```sh
# api/ (testes de integração precisam do Postgres de dev)
docker compose -f api/compose.dev.yaml up -d
export NELISA_TEST_DATABASE_URL='postgres://nelisa:nelisa@localhost:5433/postgres?sslmode=disable'
cd api
go test ./...
go vet ./...
staticcheck ./...
sqlc generate        # depois de mudar queries/ ou migrations/

# web/
cd web
npm ci
npm run lint
npm run build
```

Sem `NELISA_TEST_DATABASE_URL` os testes de integração são pulados
localmente; no CI eles são obrigatórios.

## Pronto significa

- Testes cobrindo o comportamento novo, incluindo a tentativa de violar as
  regras acima quando a issue mexer nelas.
- Todos os comandos acima limpos, rodados de verdade, não presumidos.
- `openapi.yaml` atualizado se um endpoint mudou, e o cliente TS
  regenerado.
- `CONTEXT.md` atualizado se surgiu ou mudou um termo do domínio; ADR novo se
  a decisão for difícil de reverter e surpreendente sem contexto.
- PR ligado à issue, com o checklist do template preenchido honestamente.

## Na dúvida

Pergunte na issue em vez de adivinhar. Um PR pequeno e correto vale mais que
um grande que precisa ser refeito.
