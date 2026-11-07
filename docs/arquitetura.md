# Arquitetura e padrões de projeto

Referência técnica para quem escreve código no Nelisa, pessoa ou agente. O
vocabulário está em [`CONTEXT.md`](../CONTEXT.md), a tradução para o código
em [`nomes-no-codigo.md`](nomes-no-codigo.md), o porquê das decisões em
[`adr/`](adr/) e a ordem do trabalho em [`plano.md`](plano.md).

Quando este documento e um ADR discordarem, vale o ADR. Mudar uma regra daqui
exige PR que altere este arquivo, não uma exceção silenciosa no código.

## Visão geral

```
Leitor/Equipe ──► Cloudflare ──► web/ (Next.js) ──/api──► api/ (Go) ──► Postgres
                                                           │
                                                 worker (River) ──► APIs da Câmara e do Senado
                                                                ──► API do Claude (fase 4)
```

- **Monorepo:** `api/` (Go) e `web/` (Next.js). Um único repositório, um PR
  pode tocar os dois.
- **`api/` é a única dona dos dados e das regras** (ADR 0004). O Next renderiza
  e encaminha; nunca decide visibilidade, permissão ou estado de revisão.
- **Um binário Go, três modos:** `api` (HTTP), `worker` (jobs do River) e
  `migrate` (execução única antes dos outros dois).
- **Portável para a AWS desde já:** Postgres puro (RDS), armazenamento
  S3-compatível, toda configuração por variável de ambiente, nada amarrado ao
  fogolab ou ao Komodo.

## Backend (`api/`)

### Estrutura

```
api/
├── cmd/nelisa/main.go     # lê config, monta dependências, escolhe o modo
├── internal/
│   ├── proposition/       # um diretório por módulo de domínio
│   │   ├── domain/        # entidades, value objects, regras puras
│   │   ├── app/           # casos de uso + portas (interfaces)
│   │   ├── store/         # adaptador Postgres (sqlc) das portas
│   │   │   └── dbgen/     # código gerado pelo sqlc, só deste módulo
│   │   └── httpapi/       # handlers HTTP do módulo
│   ├── review/ queue/ progress/ legislator/ ingest/
│   ├── contribution/ chamber/ team/ audit/ ai/
│   └── platform/          # infra compartilhada, sem regra de negócio:
│                          # config, db, httpx, clientip, auth, password,
│                          # clock, buildinfo, observability
├── migrations/            # goose (SQL)
├── queries/               # SQL do sqlc, um arquivo por módulo
└── openapi.yaml           # contrato da API; fonte do cliente TS
```

Os módulos são os da tabela de [`nomes-no-codigo.md`](nomes-no-codigo.md).
`team/` é o login e os usuários da equipe, portado do `identity` do open-nebula
(ADR 0001).

**Autenticação.** `platform/auth` guarda o vocabulário comum a todos
os módulos: o ator (`auth.Actor`: id e role da pessoa da equipe autenticada) levado no contexto
(`auth.ActorFrom(ctx)`), as roles (`Editor ⊂ Revisor ⊂ Admin`, mesmos valores
de `team/domain.Role`, que é a fonte da verdade), `auth.RequireRole` para
recusar cedo e o token de acesso JWT. Quem valida a sessão e põe o ator no
contexto é o middleware de `team/httpapi` (caso de uso `Authenticate`), que o
`main.go` monta na frente de toda rota de `/api/team` fora de
`/api/team/auth`. Os outros módulos só leem o ator de `platform/auth`; nunca
importam o `team`.

### Regras de dependência

- `domain/` não importa nada do projeto nem de banco, HTTP ou JSON. Só Go
  padrão. É onde ficam as regras: estados de revisão, quatro olhos, visibilidade.
- `app/` importa o `domain/` do próprio módulo e declara as **portas** que
  precisa (`type PropositionRepo interface {...}`) no próprio pacote.
- `store/` e `httpapi/` implementam e usam as portas; nunca o contrário.
- Um módulo **não importa** `domain/`, `store/` ou `httpapi/` de outro. Quando
  precisa de algo de outro módulo, declara uma porta no seu `app/` e o
  `main.go` liga a implementação.
- `platform/` não conhece nenhum módulo.

### Casos de uso

- Um caso de uso por arquivo em `app/` (`approve_review.go`), com um struct
  que recebe as portas no construtor e um método `Execute(ctx, input)`.
- Toda ação da equipe que muda conteúdo, permissão ou estado de revisão grava
  um evento de **Auditoria** na mesma transação.
- Autorização acontece no caso de uso, com o usuário vindo do contexto, e não
  só no handler. O handler pode recusar cedo; o caso de uso é quem garante.

### Banco

- **sqlc** para consultas (SQL escrito à mão em `queries/`, código gerado e
  commitado), **pgx** como driver, **goose** para migrations.
- Até a ida para a AWS há uma única migration `0001_inicial`, editável; o
  banco pode ser dropado e recriado. Depois disso, só migrations novas e
  compatíveis com a versão anterior (ver `plano.md`).
- Um schema Postgres por módulo, como no open-nebula.
- Transações abertas no caso de uso (porta `TxRunner`), nunca no handler. A
  transação viaja no contexto: `db.TxRunner.WithinTx(ctx, fn)` a abre e todo
  store obtém a conexão por `db.Conn(ctx, pool)`, que devolve a transação se
  houver uma. É assim que o caso de uso de um módulo grava a mudança e o
  evento de **Auditoria** (módulo `audit`, ligado por porta no `main.go`)
  juntos, sem um importar o store do outro.
- sqlc gera **um pacote por módulo** (`internal/<módulo>/store/dbgen`), a
  partir de `queries/<módulo>.sql` (ver `sqlc.yaml`). Cada pacote só tem as
  consultas do próprio módulo; tipos de coluna `uuid` viram `string` e
  `timestamptz` vira `time.Time`.
- Sem ORM e sem SQL montado por concatenação.

### Jobs

- **River** (fila no próprio Postgres) para Importação, Sincronização e
  rascunhos de IA. Nada de goroutine solta para trabalho que precisa
  sobreviver a um restart.
- Jobs idempotentes: rodar duas vezes não duplica dados.
- Chamadas às APIs oficiais com limite de concorrência por domínio, timeout e
  `User-Agent` identificando o Nelisa.
- Na v1 não há busca de URL arbitrária (Indicação só checa o domínio).

### HTTP

- **chi** como roteador. Contrato em `openapi.yaml`; quem muda um endpoint
  muda o contrato no mesmo PR.
- Rotas públicas em `/api/public/...`, sempre filtradas pela regra de
  visibilidade da camada pública. Rotas da equipe em `/api/team/...`,
  sempre autenticadas.
- Erros em JSON com `{ "error": { "code": "snake_case", "message": "..." } }`.
  A mensagem pode ir para a tela; detalhes internos nunca.
- Limites globais de tamanho do corpo; rate limit por IP e por usuário.
- IP do cliente: `CF-Connecting-IP` só quando a requisição vem de proxy
  confiável (`TRUSTED_PROXY_CIDRS`); senão, o IP da conexão.
- Login: JWT de acesso curto (15 min, no corpo da resposta, header
  `Authorization: Bearer`) + refresh opaco em cookie `HttpOnly; Secure;
  SameSite=Strict; Path=/api/team/auth`, girado a cada uso (reuso depois de
  30 s encerra todas as sessões). A role vem do cadastro a cada requisição,
  não do token. Como o Next faz proxy na mesma origem, não há CORS.
- O primeiro Admin nasce por `POST /api/team/auth/bootstrap`, que só funciona
  com `BOOTSTRAP_TOKEN` configurado e sem nenhum usuário cadastrado.

### Configuração e operação

- Tudo por variável de ambiente, lido uma vez em `platform/config` e validado
  no boot. Com `PUBLIC_BASE_URL` em `https`, o boot recusa segredo de exemplo.
- Nenhum domínio fixo no código: links absolutos usam `PUBLIC_BASE_URL`.
- Logs estruturados com `log/slog` em JSON. Nunca logar senha, token, cookie
  ou código de 2FA.
- `/version` responde o commit do build; `nelisa healthcheck` é o
  healthcheck do container.
- Imagem: build multi-stage, binário estático em `distroless/static:nonroot`.

### Testes

- **Domínio e casos de uso:** testes de unidade com fakes das portas (escritos
  à mão, sem framework de mock) e `clock` falso. Os fakes ficam em
  `app/fakes/` (pacote comum), para os testes de `app/` e de `httpapi/`
  usarem os mesmos; a transação falsa desfaz o estado em caso de erro, para
  provar que mudança e Auditoria andam juntas.
- **`store/`:** testes de integração contra Postgres real (testcontainers ou o
  Postgres do CI), com a migration aplicada.
- **`httpapi/`:** `httptest` cobrindo status, formato de erro e autorização.
- Toda regra do `CONTEXT.md` que proíbe algo (quem escreve não aprova,
  pendente não altera a Proposição, rascunho não é público) tem teste que
  tenta violar e falha.
- Testes em tabela quando houver vários casos; nomes de teste em inglês
  descrevendo o comportamento.
- `go test ./...`, `go vet` e `staticcheck` limpos.

### Estilo Go

- `gofmt`/`goimports`. Erros embrulhados com `fmt.Errorf("...: %w", err)`;
  erros de domínio como valores (`var ErrClaimTaken = errors.New(...)`)
  checados com `errors.Is`.
- `context.Context` como primeiro parâmetro de tudo que faz I/O.
- Sem `panic` fora de `main`. Sem variáveis globais mutáveis.
- Comentários explicam o porquê, não o quê.

## Frontend (`web/`)

- **Next.js (App Router), React, TypeScript estrito, Tailwind.**
- **Parte pública:** Server Components renderizados no servidor com cache, para
  prévia de link e SEO (ADR 0004). Toda página pública tem `title`,
  `description` e Open Graph.
- **Área da equipe (`/equipe`):** componentes de cliente; sem cache de página.
- **Dados só pela API Go**, pelo cliente TypeScript gerado do `openapi.yaml`.
  Proibido acessar o banco, guardar estado de domínio ou reimplementar regra
  no Next. Se o front precisa decidir algo, a API deve expor essa decisão
  (ex.: `canApprove: true`).
- Textos da interface em português; nomes de código em inglês.
- Acessibilidade e layout seguem `design-system/MASTER.md` (a criar quando o
  visual sair do protótipo): contraste AA, foco visível, navegação por
  teclado, alvos de toque de 44px, testado em 375px e 1280px.
- `npm run lint`, `npm run build` e testes limpos.

## Segurança (vale para tudo)

- Site público e político: trate toda entrada como hostil, inclusive das APIs
  oficiais.
- Nenhum segredo no repositório. Exemplos em `.env.example` com valores
  falsos.
- CI só em runners hospedados pelo GitHub; nunca `runs-on: self-hosted`
  (ADR 0005). Actions fixadas por SHA, `permissions:` mínimo por job, nunca
  `pull_request_target` com checkout do código do PR.
- Dependência nova só com motivo no PR; prefira a biblioteca padrão.
- IA (ADR 0003): só gera rascunho de Resumo e Estruturação, sempre revisado por
  humano; nunca Comentários. Texto vindo de documentos é dado, nunca
  instrução para o modelo.

## Commits e PRs

- Um assunto por PR, ligado a uma issue (`Closes #12`).
- Mensagens de commit no imperativo, em português ou inglês, explicando o
  porquê quando não for óbvio.
- Testes passam localmente antes do push; o CI precisa estar verde para o
  merge, e o fogolab só faz deploy de commit com CI verde.
