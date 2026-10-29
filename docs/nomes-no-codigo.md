# Nomes no código

O domínio é falado em português (ver `CONTEXT.md`); o código é em inglês.
Esta tabela é a tradução oficial — não invente outra.

## Pacotes (`api/internal/`)

| Pacote | Conceitos |
|---|---|
| `proposition/` | Proposição, Tipo, Apensação, Versão, Dispositivo, Estruturação, Comentário, Resumo, Resumo curto, Tema, Nota interna |
| `review/` | Revisão, quatro olhos, Errata, Correção menor, Selo de revisão |
| `queue/` | Fila de revisão, Posse |
| `progress/` | Tramitação, Voto nominal |
| `legislator/` | Parlamentar, Mandato, Autor |
| `ingest/` | Importação, Sincronização (Câmara, Senado) |
| `contribution/` | Contribuição, Indicação, Atualização de tramitação, Pedido de fonte oficial |
| `chamber/` | Casa legislativa, Fonte oficial |
| `team/` | Cadastro de equipe, sessão, 2FA, roles |
| `audit/` | Auditoria |
| `ai/` | Rascunho de IA (interface de provedor + implementação Claude) |

## Termos

| Domínio (pt) | Código (en) |
|---|---|
| Proposição | `Proposition` |
| Tipo (PL, PEC...) | `Kind` |
| Apensação | `AttachedTo` (na apensada) |
| Prejudicada | `Superseded` |
| Casa iniciadora | `OriginChamber` |
| Versão | `Version` |
| Versão vigente / em análise | `CurrentVersion` / `PendingVersion` |
| Dispositivo | `Provision` |
| Estruturação | `Structuring` |
| Documento integral | `OfficialText` (link) |
| Documento comentado | `AnnotatedText` |
| Comentário | `Comment` |
| Nota interna | `InternalNote` |
| Resumo / Resumo curto | `Summary` / `ShortSummary` |
| Tema | `Topic` |
| Rascunho de IA | `AIDraft` |
| Revisão (estado) | `ReviewState`: `draft` / `in_review` / `ready` |
| Selo de revisão | `ReviewBadge` |
| Segunda revisão pendente | `PendingSecondReview` |
| Errata / Correção menor | `Erratum` / `MinorFix` |
| Fila de revisão / Posse | `ReviewQueue` / `Claim` |
| Tramitação | `Progress` |
| Voto nominal | `RollCallVote` |
| Esfera | `Sphere`: `federal` / `state` / `municipal` |
| Casa legislativa | `Chamber` |
| Fonte oficial | `OfficialSource` |
| Parlamentar / Mandato / Autor | `Legislator` / `Term` / `Author` |
| Importação / Sincronização | `Import` / `Sync` |
| Contribuição | `Contribution`: `pending` / `accepted` / `rejected` |
| Indicação | `Referral` |
| Atualização de tramitação | `ProgressUpdate` |
| Pedido de fonte oficial | `SourceRequest` |
| Código de acompanhamento | `TrackingCode` |
| Leitor / Editor / Revisor / Admin | `Reader` / `Editor` / `Reviewer` / `Admin` |
| Auditoria | `Audit` |
| Autoria editorial | `EditorialAuthorship` |
| Imagem de compartilhamento | `ShareImage` |

Telas, textos e mensagens para o usuário continuam em português.
