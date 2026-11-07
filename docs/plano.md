# Plano até o deploy da v1

Documento vivo, preenchido durante o grill. Termos conforme `CONTEXT.md`.

## Marco final

**v1 no fogolab, pública para leitura, em beta discreto** (`noindex`, sem
divulgação). Sem basic auth nem Cloudflare Access na frente: a área da equipe é
protegida só pelo login próprio do Nelisa (ADR 0001). A equipe trabalha com
dados reais da Câmara e do Senado.

Tudo continua compatível com a AWS (Postgres puro → RDS, armazenamento
S3-compatível, containers configurados por env, nada amarrado ao Komodo). A
migração para a AWS é a fase seguinte e fica fora deste plano.

## Fases

Cada fase termina com deploy no fogolab.

### Fase 0 — Fundação
- `api/` em Go: chi, pgx, sqlc, goose; binário único com modos `api` e `worker`
- Postgres no stack do fogolab; `/version` na API
- Migrations: modo `migrate` do binário, rodado como container de execução
  única antes da API e do worker (`service_completed_successfully`); na AWS,
  task única antes do deploy no ECS
- Enquanto não houver dados a preservar, o schema fica numa migration inicial
  editável e o banco é dropado e recriado à vontade, inclusive no fogolab
- **Congelamento só na ida para a AWS:** a `0001_inicial` é consolidada nesse
  momento; dali em diante, só migrations novas e compatíveis com a versão
  anterior
- Consequência: no fogolab, recriar o banco a partir da fase 2 apaga trabalho
  editorial. Antes de cada recriação, `pg_dump --data-only` e recarga depois
  (ajustando à mão o que o schema novo quebrar), ou aceitar a perda
- CI de testes em runners do GitHub (ADR 0005)
- Testes passam antes do push (local) e o deploy só builda commit da `main`
  com o CI verde (remoto): o local-build consulta o status do check na API do
  GitHub e pula commits vermelhos ou em andamento
- URL pública só via env (`PUBLIC_BASE_URL`): nada de domínio fixo no código,
  para a troca de domínio ser só config
- `identity` portado; `/equipe` usa o login real e o protótipo sai do ar
- **Pronto quando:** o Admin entra pelo bootstrap e não há mais contas de teste

### Fase 1 — Proposições reais
- Importação e Sincronização das APIs da Câmara e do Senado via River
- Proposição em várias Casas e apensação (ADR 0006)
- Páginas públicas lendo do banco
- **Pronto quando:** PEC 221/2019 e PEC 8/2025 vêm das APIs, não de fixture

### Fase 2 — Edição e quatro olhos
- Resumo, documento integral versionado, Comentários por dispositivo (ADR 0002)
- Fila de revisão, posse, Segunda revisão pendente
- Regra de visibilidade aplicada na camada pública do Go
- **Backup antes de existir conteúdo editorial:** container de `pg_dump` diário
  no stack, gravando numa pasta coberta pelo Duplicati (7 diários + 4
  semanais), com um restore testado. Na AWS, o RDS assume
- **Pronto quando:** um Editor publica e um Revisor aprova

### Fase 3 — Contribuições
- Indicação (só checagem de domínio oficial), Atualização de tramitação,
  Pedido de fonte oficial
- Código de acompanhamento; moderação pela Fila de revisão
- Anti-abuso: Cloudflare Turnstile no formulário (token validado no Go);
  rate limit por IP lido de `CF-Connecting-IP` só quando a requisição vem do
  tunnel (ex.: 5/hora); teto global de pendentes que pausa o formulário
- **Pronto quando:** um Leitor anônimo contribui e acompanha pelo código

### Beta
Abre ao fim da fase 3: `noindex` mantido, dados reais revisados.

### Fase 4 — IA (ADR 0003, depois do beta)
- Rascunho de Resumo e Estruturação via API do Claude; nunca Comentários

## Login (identity)

Portar o contexto `identity` do open-nebula/carol-clima para `api/`, com base
de usuários **própria** do Nelisa (ADR 0001): JWT de acesso + refresh em cookie
`HttpOnly; Secure; SameSite=Strict` na origem do Nelisa (o Next faz proxy de
`/api`), bcrypt, goose. Roles Editor ⊂ Revisor ⊂ Admin; primeiro usuário vira
Admin pelo bootstrap; sem auto-cadastro.

## TODO

Itens decididos mas ainda não desenvolvidos (o nebula não tem):

- [ ] 2FA TOTP obrigatório para Revisor e Admin; reset de 2FA só pelo Admin
- [ ] Troca obrigatória de senha no primeiro login e após todo reset
- [ ] Senha mínima de 8 caracteres checada contra lista de senhas vazadas
- [ ] Eventos de login, reset de senha e reset de 2FA registrados na Auditoria
- [ ] Tirar do ar a tela de login do protótipo (contas de teste com senha exposta)

- [ ] Domínio oficial (o usuário decide depois); ao trocar, 301 permanente de
      `nelisa.foguinhodogoias.com` para o novo

- [ ] fogolab: gate de CI verde no local-build para **todos** os projetos
      (repos privados precisam de token de leitura na consulta)

Depois da v1 (ainda sem desenho):

- [ ] Login para leitor, com vínculo de contas de redes sociais. Pelo
      `CONTEXT.md`, Leitor é "qualquer pessoa sem login": precisa de termo
      novo e de decidir se fica separado da base da equipe (ADR 0001)
- [ ] Facilitar o compartilhamento (botões de compartilhar, imagem de
      story/preview via `next/og`)

## Decisões do grill

- 2026-11-07 — marco final = beta público no fogolab (opção A); AWS depois.
- 2026-11-07 — login = port do identity do nebula, usuários próprios; extras do ADR 0001 no TODO.
- 2026-11-07 — fases 0–4; beta ao fim da fase 3, IA depois.
- 2026-11-07 — backup via pg_dump → Duplicati na fase 2; os outros 5 bancos do fogolab são tarefa separada do fogolab.
- 2026-11-07 — anti-abuso = Turnstile + rate limit por CF-Connecting-IP + teto da Fila; login de leitor e compartilhamento no TODO pós-v1.
- 2026-11-07 — beta em nelisa.foguinhodogoias.com; domínio oficial depois, URL base por env.
- 2026-11-07 — migrations via modo migrate (one-shot); em desenvolvimento, drop/recria com migration inicial consolidada.
- 2026-11-07 — schema congela só na migração para a AWS.
- 2026-11-07 — testes passam local e remoto; deploy só com CI verde, para todos os projetos.
