# Fase 2: Correção menor e Errata (#35)

Continua o [ciclo editorial](fase-2-ciclo-editorial.md). Termos do
[`CONTEXT.md`](../../CONTEXT.md) ("Correção menor", "Errata"). Só valem para
o conteúdo editorial (Resumo, Resumo curto, Temas) da **Versão vigente** de
uma Proposição; Comentários entram quando existirem (#20), com as mesmas
regras.

Diferença essencial: **Correção menor** não muda o sentido (digitação,
formatação, link) e entra direto; **Errata** muda o sentido e só entra com
quatro olhos, ficando pública como nota datada. Nova Versão é outra coisa
(o texto da Proposição mudou); aqui quem errou foi o Nelisa.

## Correção menor (`MinorFix`)

- Editor, Revisor ou Admin aplica na Versão vigente:
  `PUT /api/team/propositions/{id}/versions/{versionId}/minor-fix`
  `{summary, short_summary, topic_ids, revision}`.
- Exige a Posse (assume se livre, 409 `claim_taken` se alheia), trava
  otimista por `revision` (409 `stale_content`), mesmas validações do
  conteúdo. Não pode haver Errata pendente na mesma Versão (409
  `erratum_pending`): as duas mexeriam no mesmo texto.
- Aplica na hora: a Proposição **segue pronta**, o Leitor vê o texto
  corrigido sem aviso. Quem corrigiu entra em `authors`.
- Gravada em `review.minor_fixes`: `id`, `proposition_id`, `version_id`,
  `fields` (quais campos mudaram), `before` (jsonb com o conteúdo anterior,
  só para a equipe conferir), `created_by`, `created_at`, `checked_by`,
  `checked_at`. Auditoria `content.minor_fix` (Versão, campos; nunca o
  texto).
- **Correção a conferir:** cada uma sem `checked_by` aparece na Fila
  (motivo `minor_fix_check`). Revisor ou Admin que **não** fez a correção a
  confere: `POST /api/team/propositions/{id}/minor-fixes/{fixId}/check`
  (Auditoria `review.minor_fix_check`). Se discordar, não há "rejeitar":
  faz outra Correção menor (desfazendo) ou abre uma Errata. A tela mostra o
  antes e o depois.
- Nenhuma Correção menor sai no público além do próprio texto corrigido.

## Errata (`Erratum`)

- Editor, Revisor ou Admin propõe, na Versão vigente:
  `POST /api/team/propositions/{id}/versions/{versionId}/errata`
  `{summary, short_summary, topic_ids, said, correct, revision}`.
  - `summary`, `short_summary`, `topic_ids`: o conteúdo completo como fica
    depois da Errata (mesmas validações do conteúdo).
  - `said` e `correct`: a nota pública, escrita pela equipe ("dizia X; o
    correto é Y"), cada uma com até 500 caracteres, ambas obrigatórias.
  - `revision`: do conteúdo vigente que a pessoa leu (409 `stale_content`).
- No máximo **uma Errata pendente por Versão** (409 `erratum_pending`).
  Não exige Posse (não edita nada até ser aprovada).
- Enquanto pendente, **nada muda** para o Leitor: a Proposição segue pronta
  com o conteúdo antigo.
- Aprovar: `POST .../errata/{erratumId}/approve` `{revision}` (a revision
  da própria Errata não muda; a do conteúdo vigente tem de ser a mesma de
  quando a Errata foi proposta, senão 409 `stale_content` e a Errata
  precisa ser refeita).
  - Revisor ou Admin, **nunca quem propôs** (403 `four_eyes`). Sem exceção
    para o Admin: a Errata corrige um erro já publicado, e a regra do
    `CONTEXT.md` pede "outro Revisor".
  - Na mesma transação: o conteúdo vigente vira o da Errata (revision sobe,
    quem propôs entra em `authors`), a Errata fica `approved` com
    `decided_by`/`decided_at`, Auditoria `review.erratum_approve`.
- Recusar: Revisor ou Admin que não propôs, com motivo interno
  (`POST .../errata/{erratumId}/reject` `{reason}`, Auditoria
  `review.erratum_reject`). Retirar: quem propôs
  (`POST .../errata/{erratumId}/withdraw`). Nos dois casos nada muda.
- Gravada em `review.errata`: `id`, `proposition_id`, `version_id`,
  `proposed` (jsonb com summary/short_summary/topic_ids), `said`,
  `correct`, `base_revision`, `state` (`pending` / `approved` /
  `rejected` / `withdrawn`), `created_by`, `created_at`, `decided_by`,
  `decided_at`, `reason`.
- Fila: Errata pendente aparece com motivo `erratum`.

## Público

- `GET /api/public/propositions/{slug}` ganha `errata`: lista das Erratas
  **aprovadas da Versão vigente**, da mais nova para a mais antiga, cada uma
  `{approved_at, said, correct}`. Nada de quem propôs, aprovou ou recusou;
  Erratas pendentes, recusadas e retiradas nunca saem. Quando uma Versão
  nova vira vigente, as Erratas da anterior deixam de aparecer junto com o
  conteúdo dela.
- Página pública: seção "Erratas" depois do Resumo, cada uma com a data:
  "Em 08/11/2026: dizia X; o correto é Y."

## Equipe

- `GET /api/team/propositions/{id}` ganha: Correções a conferir (com antes
  e depois e quem fez), Errata pendente (com o proposto e a nota), histórico
  de Erratas, e as decisões `can_minor_fix`, `can_propose_erratum`,
  `can_check_minor_fix` (por correção), `can_decide_erratum`,
  `can_withdraw_erratum`, sempre decididas pela API.
- Seção "Conteúdo" da Proposição pronta: "Corrigir (correção menor)" abre o
  editor do conteúdo vigente; "Propor Errata" abre o editor com os campos
  "Dizia" e "O correto é"; caixas para conferir correção e
  aprovar/recusar/retirar Errata, com confirmação na linha.
- Fila: os motivos novos com rótulo ("Correção a conferir", "Errata").

## Testes obrigatórios

- Correção menor em Proposição pronta: aplica, segue pronta, aparece na
  Fila; quem corrigiu tenta conferir: 403; outro Revisor confere: sai da
  Fila.
- Correção menor sem Posse alheia livre: 409; com `revision` velho: 409;
  em Versão em análise ou substituída: recusada (só a vigente).
- Errata: nada muda no público enquanto pendente; quem propôs tenta
  aprovar (inclusive Admin): 403; outro Revisor aprova: conteúdo novo e
  nota no público; segunda Errata pendente na mesma Versão: 409;
  Correção menor com Errata pendente: 409; Errata cujo conteúdo base mudou:
  aprovar dá 409.
- Erratas recusadas/retiradas e pendentes nunca aparecem no público, nem
  quem propôs ou decidiu.
- Correção, conferência e decisões de Errata gravam Auditoria na mesma
  transação (falha na Auditoria desfaz).
