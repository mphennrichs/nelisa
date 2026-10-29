# Nelisa no fogolab — primeiro deploy

O fogolab constrói a imagem **no próprio servidor** e faz o deploy sozinho
(`infra/local-build` do fogolab): a cada 3 minutos o cron espelha este repo do
GitHub para o Forgejo, builda `localhost:5000/nelisa:sha-<commit>` a partir de
`./web`, reescreve o `image:` em `fogolab/nelisa/docker-compose.yaml` e chama o
`DeployStack`. **Push na `main` = protótipo atualizado em 3 a 6 minutos.**

O stack nasce ligado. Até os passos 1 e 2 serem feitos, o Komodo tenta subir e
falha (não existe imagem ainda): é esperado.

> Repo público: o runner self-hosted `fogoserver` **nunca** é registrado aqui
> (ADR 0005). O host só builda a `main`.

## 1. Servidor (`fogoserver`)

- [ ] Criar o espelho vazio `mphennrichs/nelisa` no Forgejo.
- [ ] Garantir que a chave `fogolab_deploy` **escreve** nele (se for deploy key
  por repositório, cadastrá-la no repo novo).
- [ ] Clonar com os mesmos remotes dos outros apps:
  ```sh
  git clone git@github.com:mphennrichs/nelisa.git ~/repos/nelisa
  cd ~/repos/nelisa
  git remote rename origin github
  git remote add origin ssh://forgejo-local/mphennrichs/nelisa.git
  ```
  Não defina `core.sshCommand` no clone.

## 2. Infisical `/nelisa` (projeto `fogolab`, env `prod`)

| Chave | Valor |
|---|---|
| `NELISA_BASIC_AUTH` | saída de `htpasswd -nbB equipe '<senha>'`, **crua** (com `$` simples) |

Sem essa variável o router falha fechado (ninguém entra).

## 3. Ligar

1. [ ] Esperar o `gen-stacks` declarar o Stack `nelisa` (aparece no Komodo).
2. [ ] Rodar o workflow **`sync-secrets-deploy`** do fogolab para `nelisa`.
3. [ ] Esperar o próximo ciclo do cron: ele builda, faz o bump e o deploy.
   Log: `~/.cache/fogolab-build/build.log`.
4. [ ] `docker restart server-companion-1` para criar o CNAME
   `nelisa.foguinhodogoias.com` (o companion só cria registro de container que
   está rodando).

## 4. Conferir

- [ ] `curl -u equipe:<senha> https://nelisa.foguinhodogoias.com/version`
  mostra o commit.
