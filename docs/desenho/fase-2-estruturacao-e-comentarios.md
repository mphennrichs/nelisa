# Fase 2: Estruturação e Comentários (#20)

Fecha a fase 2 depois do [ciclo editorial](fase-2-ciclo-editorial.md) e da
[Correção menor e Errata](fase-2-correcao-e-errata.md). Termos do
[`CONTEXT.md`](../../CONTEXT.md) ("Estruturação", "Dispositivo",
"Comentário", "Documento comentado"), nomes de
[`nomes-no-codigo.md`](../nomes-no-codigo.md) (`Structuring`, `Provision`,
`Comment`, `AnnotatedText`), [ADR 0002](../adr/0002-comentarios-por-dispositivo.md).

**Pronto quando:** a Importação da PEC 221/2019 busca sozinha o PDF da
Câmara, o Nelisa o quebra em Dispositivos, o Editor corrige o que errou,
escreve Comentários em dois artigos, um Revisor aprova e o Leitor vê o
Documento comentado na página da Proposição.

## Fora deste desenho

- **Snapshot** (cópia guardada do PDF, hash, Verificação de fonte, Mudança
  detectada): segue fora da v1. O PDF é baixado, lido e descartado; fica só o
  texto extraído (interno) e os Dispositivos.
- **OCR** de PDF escaneado.
- DOCX e HTML: as duas Fontes com API (Câmara e Senado) entregam PDF.
- Correção menor e Errata **de Comentário**: issue própria logo depois,
  reaproveitando o que a #35 fez para o Resumo.
- Trecho dentro do Dispositivo (âncora num pedaço do texto): o modelo já
  guarda, mas a tela da v1 comenta o Dispositivo inteiro. Ver "Decisões".

## 1. Busca do PDF

- Toda Versão nova (a 1 da Importação ou a criada pelo Editor) agenda um job
  River `structuring.fetch` com o `official_text_url` da Versão.
- **Só busca em host permitido** (lista fechada no código: `camara.leg.br`,
  `www.camara.leg.br`, `legis.senado.leg.br`, `www25.senado.leg.br` e os que
  o spike mostrar), por https, seguindo no máximo 3 redirecionamentos e
  checando o host a cada um. Fora da lista, a Estruturação fica
  `manual` (abaixo). Isso é o que o `CONTEXT.md` pede ("só busca sozinho em
  fontes com API cadastrada") e fecha a porta para SSRF pelo link que o
  Editor digita.
- Limites: 20 MB, 60 s, conteúdo tem de começar com `%PDF`. User-Agent do
  Nelisa, sem dados de ninguém.
- Falha de rede ou 5xx: River tenta de novo (backoff, 5 tentativas). 404,
  host fora da lista, não é PDF, grande demais: falha definitiva.

## 2. Extração do texto

- **Recomendação:** PDFium rodando em WebAssembly dentro do Go
  (`klippa-app/go-pdfium`, modo `webassembly`, via wazero). Sem cgo, cabe na
  imagem `distroless/static` de hoje, e o PDF (entrada não confiável) é
  lido dentro de uma sandbox de memória, não num processo com acesso ao
  sistema. Custo: binário uns 5 MB maior e extração mais lenta (irrelevante,
  é job).
- Alternativa: `pdftotext` (poppler) chamado por `exec`. Melhor conhecido,
  mas troca a imagem base por uma com libc e poppler (mais superfície, CVEs
  de poppler são frequentes) e precisa de limite de recurso no processo.
- **Spike antes de implementar** (meio dia): extrair 6 PDFs reais (2 PL e 1
  PEC da Câmara, 2 PL e 1 PEC do Senado) com as duas opções e comparar ordem
  de leitura, hifenização, cabeçalhos e rodapés. Escolhe-se a que acertar mais
  artigos sem conserto; se empatar, a WebAssembly.
- Limpeza depois de extrair: junta palavras hifenizadas no fim da linha;
  remove linhas que se repetem em mais da metade das páginas (cabeçalho e
  rodapé, "Página 2 de 7", o código de autenticação `*CD26…*` da Câmara);
  normaliza espaços e travessões (`–`, `—`, `-`).
- PDF sem texto (escaneado): falha com motivo `no_text`. A Estruturação fica
  `manual`.

## 3. Parser (texto → Dispositivos)

Regras da LC 95/98, por expressão regular sobre linhas, gerando uma árvore:

| Dispositivo | início da linha | endereço |
|---|---|---|
| Ementa | o parágrafo antes do primeiro artigo, depois do título | `ementa` |
| Artigo | `Art. 1º`, `Art. 10.`, `Art. 3º-A` | `art-3a` |
| Parágrafo | `§ 2º`, `Parágrafo único.` | `art-3.par-2`, `art-3.par-unico` |
| Inciso | `IV –` (romano + travessão) | `art-3.par-2.inc-4` |
| Alínea | `b)` | `…inc-4.ali-b` |
| Item | `1.` dentro de alínea | `…ali-b.item-1` |
| Justificação | `JUSTIFICAÇÃO`, `JUSTIFICATIVA` até o fim | `justificacao` |

- O endereço é estável dentro da Versão e legível ("Art. 3º, § 2º, IV" é o
  rótulo exibido; `art-3.par-2.inc-4` é a chave).
- **Texto citado entre aspas** (PEC e PL que alteram outra lei: `"Art. 5º
  …"`) **não vira Dispositivo**: fica como texto do Dispositivo que o cita.
  O Comentário vai no artigo da Proposição ("Art. 1º"), não no artigo da lei
  alterada. Sem isso o parser confunde os dois.
- O que não casa com nenhuma regra é continuação do Dispositivo anterior.
- Endereço repetido (erro do PDF ou do parser) não quebra: ganha sufixo
  (`art-3~2`) e a Estruturação sai com aviso para o Editor.
- Testes do parser com o texto extraído dos 6 PDFs do spike guardado em
  `testdata/` (texto, não o PDF).

## 4. Modelo (`proposition/`)

**Estruturação** (`proposition.structurings`, 1:1 com a Versão):

| campo | regra |
|---|---|
| `version_id` (PK) | |
| `state` | `fetching` → `ready_to_check` → `confirmed`; ou `manual` |
| `failure` | `host_not_allowed`, `not_found`, `not_pdf`, `too_large`, `no_text`, `network` (só em `manual`) |
| `source_text` | texto extraído e limpo; **interno**, nunca sai no público |
| `warnings` | lista (endereço repetido, artigo fora de ordem, ementa não achada) |
| `revision`, `updated_by`, `updated_at`, `confirmed_by`, `confirmed_at` | |

**Dispositivo** (`proposition.provisions`): `version_id`, `address`
(único por Versão), `parent_address`, `kind` (`ementa`, `article`,
`paragraph`, `item`, `subitem`, `point`, `justification`), `label` ("Art. 3º,
§ 2º, IV"), `text`, `position` (ordem de leitura), `text_hash`.

**Comentário** (`proposition.comments`): `id`, `version_id`,
`provision_address`, `body` (texto simples com parágrafos, ≤ 3.000),
`quote_start`/`quote_end` (nulos na v1, ver "Decisões"),
`needs_confirmation`, `authors` (`uuid[]`), `revision`, `created_by`,
`created_at`, `updated_by`, `updated_at`, `deleted_at`.

## 5. Fluxo do Editor

- **Corrigir a Estruturação** = editar o `source_text` e reprocessar. A tela
  mostra o texto à esquerda e a árvore resultante à direita; o Editor arruma
  a quebra de linha, um travessão, um artigo grudado, e manda reprocessar.
  Não há edição da árvore à mão: o parser é a única forma de gerar
  Dispositivos, e o texto corrigido fica como registro do que foi mudado.
- **`manual`** (busca ou extração falhou): a tela diz o motivo e oferece
  "colar o texto" (o mesmo editor de `source_text`, começando vazio) e o link
  oficial para o Editor copiar de lá. É a única porta para colar texto; com
  PDF bom, o texto vem sozinho.
- Reprocessar exige Posse e `revision` (409 `stale_content`), só na Versão
  `pending`. Se o novo texto **some com um endereço que tem Comentário**:
  409 `anchors_removed` com a lista. O Editor move ou apaga esses
  Comentários antes (a âncora nunca fica órfã).
- **Confirmar a Estruturação** (`confirmed`): ação explícita, entra em
  `authors` do conteúdo. Comentar só depois de confirmada (para não comentar
  em cima de árvore que ainda vai mudar); reprocessar depois de confirmada
  volta para `ready_to_check`.
- **Comentários:** criar, editar, apagar (soft) na Versão `pending`, com
  Posse e `revision`, como o Resumo. Quem grava entra em `authors` do
  Comentário **e** do conteúdo da Versão (os quatro olhos valem para tudo que
  o aprovador vai assinar).
- Auditoria: `structuring.fetch_failed`, `structuring.reprocess`,
  `structuring.confirm`, `comment.create`, `comment.update`,
  `comment.delete` (metadado, nunca o texto).

## 6. Aprovação

`approve_blockers` ganha:

- `structuring_pending`: Estruturação em `fetching` ou `ready_to_check`.
- `comments_unconfirmed`: algum Comentário com `needs_confirmation`.

Estruturação `manual` sem texto colado **não bloqueia**: a Proposição pode ir
a "pronto" só com Resumo, e o Leitor vê "documento comentado indisponível,
veja o texto oficial". Comentários não são obrigatórios.

## 7. Versão nova

- A Versão nova ganha a própria Estruturação (busca de novo).
- Quando a Versão nova é **confirmada** na Estruturação, cada Comentário da
  Versão vigente cujo Dispositivo existe na nova com o mesmo `address` **e**
  o mesmo `text_hash` é copiado como `needs_confirmation`. Os outros ficam só
  na Versão antiga (a equipe vê o histórico; o Leitor não).
- "Confirmar comentário trazido" é ação explícita, como o Resumo.

## 8. Público

- `GET /api/public/propositions/{slug}` ganha `annotated_text`: `null` sem
  Versão vigente ou sem Estruturação confirmada; senão lista ordenada de
  `{address, label, kind, text, depth, comments: [{body}]}` da Versão
  vigente.
- Nunca sai: `source_text`, `warnings`, autores, Comentários apagados, de
  Versão `pending` ou `needs_confirmation`, Estruturação de Versão em
  análise. Testes que tentam vazar cada um.
- Página pública: seção "Documento comentado" depois do Resumo e das
  Erratas. Cada Dispositivo com Comentário tem o Comentário logo abaixo,
  recuado e marcado ("Comentário do Nelisa"); link âncora por endereço
  (`#art-3.par-2`) para compartilhar. Dispositivos sem Comentário
  aparecem em texto corrido. Sem download de PDF.

## 9. API da equipe

- `GET /api/team/propositions/{id}`: Estruturação da Versão `pending` (e da
  vigente), Dispositivos, Comentários, `can_edit_structuring`,
  `can_comment`.
- `PUT .../versions/{v}/structuring` `{source_text, revision}` (reprocessa).
- `POST .../versions/{v}/structuring/confirm` `{revision}`.
- `POST .../versions/{v}/structuring/retry` (refaz a busca, só em `manual`
  por `network`).
- `POST .../versions/{v}/comments` `{provision_address, body}`;
  `PUT|DELETE .../comments/{commentId}` `{body, revision}`;
  `POST .../comments/{commentId}/confirm`.

## 10. Ordem de implementação

1. Spike da extração (escolhe a biblioteca) e texto dos 6 PDFs em
   `testdata/`.
2. Parser puro (`proposition/domain/structuring`), testado só com texto.
3. Busca + extração + job River + estados + `manual`.
4. Tela de Estruturação (texto ↔ árvore, reprocessar, confirmar).
5. Comentários (equipe), bloqueios de aprovação, cópia para Versão nova.
6. Público: `annotated_text` e a seção na página.

## Decisões (confirmadas em 2026-11-08)

1. **Biblioteca de PDF:** PDFium em WebAssembly (recomendado) ou
   `pdftotext`, decidido pelo spike. Vira ADR se a escolha for
   `pdftotext` (muda a imagem base).
2. **Colar texto quando a busca falha:** recomendado sim, só nesse caso
   (escaneado, host fora da lista). Sem isso uma Proposição municipal ou um
   PDF escaneado nunca tem Documento comentado.
3. **Trecho:** recomendado guardar as colunas e deixar a tela para depois;
   comentar o Dispositivo inteiro cobre a v1.
4. **Correção menor/Errata de Comentário** em issue separada, logo depois.
