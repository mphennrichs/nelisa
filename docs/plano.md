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
- CI de testes em runners do GitHub (ADR 0005)
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
- **Pronto quando:** um Editor publica e um Revisor aprova

### Fase 3 — Contribuições
- Indicação (só checagem de domínio oficial), Atualização de tramitação,
  Pedido de fonte oficial
- Código de acompanhamento; moderação pela Fila de revisão; rate limit
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

## Decisões do grill

- 2026-11-07 — marco final = beta público no fogolab (opção A); AWS depois.
- 2026-11-07 — login = port do identity do nebula, usuários próprios; extras do ADR 0001 no TODO.
- 2026-11-07 — fases 0–4; beta ao fim da fase 3, IA depois.
