# Nelisa no fogolab — deploy

O fogolab constrói as imagens **no próprio servidor** e faz o deploy sozinho
(`infra/local-build` do fogolab): a cada 3 minutos o cron espelha este repo do
GitHub para o Forgejo e, para o commit da `main`, builda
`localhost:5000/nelisa:sha-<commit>` (de `./web`) e
`localhost:5000/nelisa-api:sha-<commit>` (de `./api`), reescreve os `image:` em
`fogolab/nelisa/docker-compose.yaml` e chama o `DeployStack`.

**Só commit com CI verde vai para o ar.** As duas entradas do `builds.json` têm
`require_ci: true`: antes de buildar, o host consulta os check runs do commit
na API pública do GitHub e só segue se todos os jobs do workflow `CI`
terminaram com `success`. CI falhou, ainda rodando, API fora ou rate limit = o
ciclo pula e tenta de novo 3 minutos depois (motivo no log). Push na `main` com
CI verde = no ar em uns 3 a 6 minutos depois do CI terminar.

> Repo público: o runner self-hosted `fogoserver` **nunca** é registrado aqui
> (ADR 0005). O host só builda a `main`.

## Stack

| Serviço    | Imagem                         | Rede              | Papel |
|------------|--------------------------------|-------------------|-------|
| `postgres` | `postgres:17-alpine`           | interna           | banco, volume nomeado `postgres_data`, sem porta publicada |
| `migrate`  | `nelisa-api`, `migrate`        | interna           | aplica as migrations e sai (`restart: "no"`) |
| `api`      | `nelisa-api`, `api`            | interna           | HTTP em `http://api:8080`; fora do Traefik |
| `worker`   | `nelisa-api`, `worker`         | interna           | jobs do River: Sincronização com a Câmara e o Senado (fase 1) |
| `app`      | `nelisa` (web)                 | traefik + interna | Next.js público; `API_INTERNAL_URL=http://api:8080` |

Ordem de subida: `postgres` saudável → `migrate` termina com sucesso →
`api` e `worker`. O `migrate` aplica também as migrations do River (schema
`river`); não há container separado para isso.

`SYNC_INTERVAL` (opcional, padrão `6h`, mínimo `1m`) é de quanto em quanto
tempo o worker sincroniza todos os Vínculos de fonte confirmados.

`CONTRIBUTIONS_ENABLED` (opcional, padrão desligado) liga o envio de
Contribuições pelo Leitor (`true`). Fica desligado até o Turnstile e o rate
limit da fase 3 estarem no ar; a consulta pelo código funciona sempre.

## Primeiro deploy da api (fase 0)

Feito **nesta ordem**. Enquanto o passo 3 não acontecer, nada muda no ar: o
protótipo segue rodando.

### 1. CI verde na `main`

- [ ] Fazer merge da branch `fase-0/ci` (o workflow `.github/workflows/ci.yml`)
  na `main` e conferir que o CI ficou verde nesse commit
  (`gh run list --branch main`). Sem isso o gate do host nunca deixa buildar:
  commit sem check runs conta como "não verde".

### 2. Segredos no Infisical

Em `secrets.foguinhodogoias.com`, projeto `fogolab`, ambiente `prod`, pasta
`/nelisa`:

| Variável            | Obrigatória | Como gerar | Para quê |
|---------------------|-------------|------------|----------|
| `POSTGRES_PASSWORD` | sim | `openssl rand -hex 32` | senha do Postgres; o compose monta a `DATABASE_URL` com ela |
| `JWT_SECRET`        | sim | `openssl rand -base64 48` | assina os tokens de acesso (mínimo 32 bytes; a api recusa valor de exemplo) |
| `BOOTSTRAP_TOKEN`   | só no primeiro boot | `openssl rand -base64 48` | cria o primeiro Admin; apagar depois (passo 5) |

- `POSTGRES_PASSWORD` em **hex** de propósito: ela entra na URL
  `postgres://nelisa:<senha>@postgres:5432/nelisa`, e base64 tem `/`, `+` e
  `=`, que quebram a URL.
- **Não** crie `DATABASE_URL` no Infisical: o compose deriva ela da senha, para
  não existirem duas cópias que podem divergir.
- `PUBLIC_BASE_URL` (`https://nelisa.foguinhodogoias.com`) e
  `API_INTERNAL_URL` estão fixos no compose; não precisam de segredo.
- A senha do Postgres só vale na **primeira** inicialização do volume. Trocar
  depois exige `ALTER USER` dentro do banco (ou apagar o volume enquanto não
  houver dado real).
- Se ainda existir `NELISA_BASIC_AUTH` na pasta, pode apagar: não é mais usado.

### 3. Push do fogolab

- [ ] No clone local do fogolab: `git pull --rebase origin main` (o bot de
  bump commita lá o tempo todo) e `git push`. O commit traz o compose novo e a
  entrada `nelisa-api` + `require_ci` no `builds.json`.
- [ ] Rodar o workflow **`sync-secrets-deploy`** do fogolab para `nelisa`
  (escreve os segredos no Env do Stack e faz o deploy).

  O primeiro deploy pode falhar por imagem inexistente
  (`localhost:5000/nelisa-api:sha-...` ainda não foi buildada): é esperado.
  Sem os segredos o compose recusa subir com a mensagem
  `defina POSTGRES_PASSWORD no Infisical /nelisa`, e o stack antigo continua
  no ar.

### 4. Esperar o deploy

- [ ] Esperar o próximo ciclo do cron: ele confere o CI, builda as duas
  imagens, faz o bump e o deploy. Log no host:
  `~/.cache/fogolab-build/build.log` (procure `[nelisa-api]`; `aguardando CI
  verde` diz por que pulou).
- [ ] Conferir: `curl https://nelisa.foguinhodogoias.com/version` mostra o
  commit novo. No host, `docker ps --filter name=nelisa` mostra `postgres`,
  `api` e `worker` de pé e `nelisa-migrate` como `Exited (0)`.

### 5. Primeiro Admin

- [ ] Criar o primeiro Admin com o `BOOTSTRAP_TOKEN` (fluxo da issue do
  módulo `team`).
- [ ] Apagar `BOOTSTRAP_TOKEN` do Infisical e rodar `sync-secrets-deploy` de
  novo para tirá-lo do container.

## Pendências conhecidas

- **Backup:** o volume `postgres_data` fica fora de `/home/fogo/docker_volumes`,
  então o Duplicati não o vê. Antes de ter dado real, falta um `pg_dump`
  diário para dentro de `docker_volumes`.
- **Egress da api e do worker (fase 1):** a rede interna não tem saída para a
  internet, e os dois precisam das APIs da Câmara
  (`dadosabertos.camara.leg.br`) e do Senado (`legis.senado.leg.br`): a
  Importação roda **na api**, dentro da requisição do Editor, e a
  Sincronização roda no worker. Os dois ganham uma rede com saída.
- **Rate limit da API do GitHub:** o gate faz 1 chamada sem token por commit
  por ciclo (até ~20/hora enquanto espera o CI), dentro do limite de 60/hora
  por IP. Se o host passar a fazer outras chamadas sem token, considerar um
  token só-leitura.
