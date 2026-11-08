# Fase 2: ciclo editorial (Versão, Resumo, Temas, Fila, Posse, quatro olhos)

Desenho das issues #19, #21 e #22, e da nova issue de Temas. Termos do
[`CONTEXT.md`](../../CONTEXT.md), nomes de
[`nomes-no-codigo.md`](../nomes-no-codigo.md), regras de
[`arquitetura.md`](../arquitetura.md). Correção menor e Errata vêm em seguida
(issue própria); Estruturação e Comentários (#20) depois, com o PDF oficial
buscado sozinho.

**Pronto quando:** um Editor escreve Resumo, Resumo curto e Temas de uma
Proposição, um Revisor (outra pessoa) aprova, e o Leitor vê tudo com o Selo
"revisado por 2 pessoas".

## Renome antes de começar

Hoje `Summary` na Proposição é a **ementa oficial**. Pela tabela de nomes,
`Summary` é o **Resumo** do Nelisa. Renomear o campo oficial para
`OfficialSummary` (coluna `official_summary`, JSON `official_summary`) em
domínio, store, ingest, openapi e web, antes de criar o Resumo.

## Modelo

### `proposition/`: Versão, conteúdo editorial, Temas

**Versão** (`proposition.versions`):

| campo | regra |
|---|---|
| `id`, `proposition_id` | |
| `seq` | 1, 2, 3… por Proposição |
| `label` | "Texto original", "Substitutivo da CCJ"… (texto livre, obrigatório, ≤ 120) |
| `official_text_url` | link http(s) do Documento integral desta Versão |
| `state` | `pending` (em análise) → `current` (vigente) → `superseded` |
| `created_by`, `created_at` | `created_by` nulo quando veio da Importação |

- No máximo uma `pending` e uma `current` por Proposição (índices únicos
  parciais).
- A Importação cria a Versão 1 (`pending`, "Texto original", com o
  `official_text_url` da fonte).
- O Editor cria Versão nova à mão (link + rótulo): só se não houver
  `pending`. A Proposição volta para `in_review` (se estava `ready`). O
  conteúdo editorial da Versão vigente é copiado para a nova marcado
  **a confirmar** (CONTEXT.md, "Versão vigente / em análise").
- `official_text_url` da Proposição (dado oficial da fonte) segue existindo;
  o link do Documento integral que o Leitor vê é o da Versão vigente (ou o
  oficial, se ainda não há vigente).

**Conteúdo editorial** (`proposition.version_contents`, 1:1 com a Versão):

| campo | regra |
|---|---|
| `summary` | Resumo, texto simples com parágrafos, ≤ 5.000 caracteres |
| `short_summary` | Resumo curto, ≤ 280 caracteres, nunca corte automático |
| `topic_ids` | 1 a 3 Temas ativos (exigido só para aprovar) |
| `needs_confirmation` | true quando veio copiado de Versão anterior |
| `authors` | `uuid[]`: todo mundo que já gravou este conteúdo (Autoria editorial) |
| `revision` | inteiro, sobe a cada gravação (trava otimista: grava com o `revision` lido, senão `409 stale_content`) |
| `updated_by`, `updated_at` | |

- Gravar o conteúdo exige a **Posse** (abaixo), adiciona quem gravou a
  `authors` e, se `needs_confirmation`, o próprio salvar não confirma: há uma
  ação explícita "confirmar conteúdo trazido" (também entra em `authors`).
- Só a Versão `pending` é editável. Conteúdo de Versão `current` só muda por
  Correção menor/Errata (próxima issue).
- Auditoria: `content.update` (metadado: Versão, campos alterados, nunca o
  texto inteiro), `version.create`, `content.confirm`.

**Tema** (`proposition.topics`): lista fechada mantida pelo Admin.
`id`, `name` (único sem diferenciar maiúsculas, ≤ 60), `active`,
`created_at`. Admin cria, renomeia e desativa (desativado some da escolha,
continua nas Versões que já o usam). Auditoria: `topic.create`,
`topic.rename`, `topic.deactivate`, `topic.reactivate`. Leitura da lista
para toda a equipe.

### `review/`: aprovação, quatro olhos, Selo

**Aprovação** (`review.approvals`, uma por Versão aprovada):
`version_id` (único), `proposition_id`, `approved_by`, `approved_at`,
`four_eyes` (bool), `second_review_by`, `second_review_at`.

Aprovar (marcar pronto) a Versão `pending`:

1. Role Revisor ou Admin. Não exige Posse.
2. Conteúdo completo: Resumo e Resumo curto não vazios, 1 a 3 Temas ativos,
   `needs_confirmation = false`. Senão `409 content_incomplete` com a lista
   do que falta.
3. **Quatro olhos:** quem está em `authors` não aprova
   (`403 four_eyes`). Exceção: o **Admin** pode, e aí `four_eyes = false`,
   o que cria a **Segunda revisão pendente**.
4. Na mesma transação: Versão vira `current`, a vigente anterior vira
   `superseded`, Proposição vira `ready`, a Posse é solta, Auditoria
   `review.approve` (Versão, `four_eyes`).

Confirmar segunda revisão: Revisor ou Admin que **não** está em `authors` e
não é quem aprovou. Grava `second_review_*`, Auditoria
`review.second_review`.

**Selo de revisão** (`ReviewBadge`): `2` se `four_eyes` ou segunda revisão
feita, senão `1`. O Leitor vê "revisado por 2 pessoas" ou "revisado por
1 pessoa", nunca nomes.

### `queue/`: Fila de revisão e Posse

**Posse** (`queue.claims`, uma por Proposição): `proposition_id` (PK),
`user_id`, `claimed_at`, `last_activity_at`.

- Editor, Revisor ou Admin **assume** (`POST .../claim`). Se outra pessoa
  tem a posse e não expirou: `409 claim_taken` com o nome de quem tem.
- **Expira** com 3 dias sem atividade (`last_activity_at`); expirada conta
  como livre.
- Gravar conteúdo **sem** posse assume automaticamente se estiver livre;
  se estiver com outra pessoa, `409 claim_taken`. Cada gravação renova
  `last_activity_at`.
- Soltar: quem tem a posse, ou Revisor/Admin (de qualquer um; Auditoria
  `claim.release` quando não é o próprio).
- Conta desativada perde as posses (a desativação solta todas; porta do
  `team` para o `queue`, ou o `queue` ignora posse de conta inativa ao
  checar: escolher a mais simples e testar).

**Fila de revisão** (`GET /api/team/queue`, toda a equipe), itens:

- Proposições `in_review` (com Versão `pending`);
- Proposições `ready` com **Segunda revisão pendente**.

Ordem: Esfera (federal > estadual > municipal), depois votação mais próxima
(ainda não há dado de votação marcada: todos caem em "sem votação"), depois
mais antigo na fila primeiro (desde quando a Versão `pending` existe, ou
desde a aprovação sem quatro olhos). Filtro `?sphere=`. Cada item traz:
nome, Esfera, motivo (`review` / `second_review`), Versão, quem tem a posse
(nome) e desde quando, e o que falta para aprovar.

## Visibilidade (#22)

A regra continua só no Go (`proposition/domain/visibility.go`), estendida:

| estado | o Leitor vê |
|---|---|
| `draft` | 404 |
| sem Versão vigente (`in_review`) | "em análise": só dados oficiais, como hoje |
| com Versão vigente | Resumo, Resumo curto, Temas e Selo **da Versão vigente**, mais os dados oficiais |
| vigente + Versão `pending` | o mesmo, mais o aviso "há texto novo em análise" com o link oficial da Versão nova |

Nunca sai: conteúdo da Versão `pending`, `authors`, quem aprovou, Posse,
`needs_confirmation`, `revision`. Testes que tentam vazar cada um.
`generateMetadata` da página pública usa o Resumo curto quando existe.

## API (contrato no `openapi.yaml`)

Equipe:

- `GET /api/team/propositions/{id}` passa a trazer Versões, conteúdo da
  `pending` (e da `current`), Posse, Selo, e `can_edit` / `can_approve` /
  `approve_blockers` decididos pela API (o front não decide).
- `POST /api/team/propositions/{id}/versions` `{label, official_text_url}`.
- `PUT /api/team/propositions/{id}/versions/{versionId}/content`
  `{summary, short_summary, topic_ids, revision}`.
- `POST .../versions/{versionId}/content/confirm`.
- `POST .../versions/{versionId}/approve`.
- `POST .../versions/{versionId}/second-review`.
- `POST /api/team/propositions/{id}/claim`, `DELETE .../claim`.
- `GET /api/team/queue[?sphere=]`.
- `GET /api/team/topics`; Admin: `POST /api/team/topics`,
  `PATCH /api/team/topics/{id}` `{name?, active?}`.

Público: `GET /api/public/propositions[/{slug}]` ganha `summary`,
`short_summary`, `topics` (nomes), `review_badge` (1 ou 2), `pending_version`
(`{label, official_text_url}` ou null), todos nulos/vazios sem Versão vigente.

## Web

- `/equipe/fila`: a Fila, com filtro por Esfera; vira a página inicial da
  equipe.
- `/equipe/proposicoes/[id]`: aba/seção "Conteúdo" com Versões, editor de
  Resumo (contador de caracteres), Resumo curto (contador 280), Temas
  (checkbox, máx. 3), Posse (assumir/soltar, quem tem), aprovar e segunda
  revisão conforme `can_*`, aviso de conflito em `stale_content`.
- `/equipe/temas` (Admin): lista, criar, renomear, desativar.
- Página pública: Resumo, Temas, Selo, aviso de texto novo em análise;
  `og:description` com o Resumo curto.

## Testes obrigatórios (além dos de sempre)

- Autor do Resumo tenta aprovar: `403 four_eyes`. Admin autor aprova:
  `four_eyes=false`, Fila mostra Segunda revisão pendente, Selo 1; outro
  Revisor confirma: Selo 2. O próprio aprovador tenta a segunda: recusado.
- Editor (não Revisor) tenta aprovar: 403.
- Aprovar com conteúdo incompleto ou `needs_confirmation`: 409.
- Segunda pessoa grava conteúdo com posse alheia: 409; com posse expirada:
  assume.
- Gravação com `revision` velho: 409, nada muda.
- Nada da Versão `pending` aparece no público, nem com uma `current`
  existindo.
- Aprovação e Auditoria andam juntas (falha na Auditoria desfaz tudo).
