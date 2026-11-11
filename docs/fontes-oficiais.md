# Fontes oficiais: o que as APIs da Câmara e do Senado oferecem

Levantamento das APIs de dados abertos das duas Casas: o que o Nelisa usa,
o que dá para buscar e ainda não usamos, e as pegadinhas já encontradas.
Termos conforme `CONTEXT.md`. Conferido nas APIs em 2026-11-08 (Câmara
v2 0.4.350; Senado OpenAPI 3.1), com a PEC 221/2019 (Câmara 2233802) e o
PL 2338/2023 (Senado 8441243) como exemplos. Os números de exemplo mudam
com o tempo.

Código: `api/internal/ingest/sources/{camara,senado}` (contrato em
`api/internal/ingest/app/source.go`) e, para o texto integral,
`api/internal/proposition/officialtext`.

## Resumo

| | Câmara | Senado |
|---|---|---|
| Raiz | `https://dadosabertos.camara.leg.br/api/v2` | `https://legis.senado.leg.br/dadosabertos` |
| Autenticação | nenhuma | nenhuma |
| Formato | JSON (`Accept: application/json`), também XML/CSV | JSON (`Accept` ou sufixo `.json`), também XML/CSV |
| Limite | listas: 15 itens por padrão, 100 no máximo; paginação por `links rel=next` | 429 acima de 10 requisições por segundo |
| Aviso de mudança | não há webhook nem feed: só consulta periódica | idem |
| Texto da Proposição | só PDF (`urlInteiroTeor`) | só PDF (`documento.url`) |
| Especificação | `/api/v2/api-docs` | `/dadosabertos/v3/api-docs` |

O que as duas não têm:
- **Texto estruturado.** Não há artigo, parágrafo nem inciso em JSON, por
  isso existe a Estruturação: busca o PDF, extrai o texto, o parser monta
  os Dispositivos e o Editor confere (ADR 0002).
- **Aviso de mudança.** Não há webhook nem feed. A Sincronização roda a cada
  `SYNC_INTERVAL` (padrão 6 h) e relê todos os Vínculos de fonte
  confirmados.

## O que o Nelisa usa hoje

### Câmara

| Endpoint | Para quê | Campos usados |
|---|---|---|
| `GET /proposicoes?siglaTipo=&numero=&ano=` | Importação: achar pelo nome oficial | `id`, `siglaTipo`, `numero`, `ano` (só aceita o nome exato) |
| `GET /proposicoes/{id}` | dados da Proposição | `ementa`, `dataApresentacao`, `urlInteiroTeor`, `uriPropPrincipal` (Apensação), `statusProposicao.descricaoSituacao` (situação) |
| `GET /proposicoes/{id}/autores` | Autoria e papel da Casa | `nome`, `ordemAssinatura`, `proponente` |
| `GET /proposicoes/{id}/tramitacoes` | Tramitação | `dataHora`, `sequencia`, `codTipoTramitacao`, `siglaOrgao`, `descricaoTramitacao`, `despacho` |
| `urlInteiroTeor` (`www.camara.leg.br/proposicoesWeb/prop_mostrarintegra?codteor=`) | Estruturação | PDF do texto integral |

Página para o Leitor conferir: `www.camara.leg.br/proposicoesWeb/fichadetramitacao?idProposicao={id}`.

### Senado

| Endpoint | Para quê | Campos usados |
|---|---|---|
| `GET /processo?sigla=&numero=&ano=` | Importação: achar pelo nome oficial | `id`, `identificacao` |
| `GET /processo/{id}` | dados e Tramitação numa chamada só | `conteudo.ementa`, `documento` (data, `url`, autoria), `objetivo` + `siglaCasaIniciadora` (papel), `situacaoAtual`, `autuacoes[].informesLegislativos` (Tramitação) |
| `documento.url` (`legis.senado.gov.br/sdleg-getter/...`) | Estruturação | PDF do texto |

A chave é o `id` do processo, não o `codigoMateria` (código do sistema
antigo, usado só para montar a página
`www25.senado.leg.br/web/atividade/materias/-/materia/{codigoMateria}`).
Os `/materia/*` estão em descontinuação; o Nelisa não usa nenhum.

### Texto integral (Estruturação)

O Nelisa só busca sozinho links `https` destes hosts: `camara.leg.br`,
`www.camara.leg.br`, `legis.senado.leg.br`, `legis.senado.gov.br`,
`www25.senado.leg.br` e `www.senado.leg.br`. Os redirecionamentos também
precisam ficar dentro dessa lista, no máximo 3.

A resposta tem de ser PDF (`%PDF-` no início) de até 20 MB. Fora disso a
Estruturação fica manual e o Editor cola o texto.

Os campos `texto` e `justificativa` de `/proposicoes/{id}` da Câmara existem,
mas vieram `null` em todos os casos testados.

## O que dá para buscar e ainda não usamos

Organizado pelo que serviria ao Nelisa. Nada aqui está decidido: cada item
precisa de issue e desenho antes de entrar.

### Sincronização mais leve

- **Câmara:** `GET /proposicoes?dataInicio=&dataFim=` lista as Proposições
  que tramitaram no período. Com isso dá para sincronizar só o que mudou, em
  vez de reler todos os Vínculos de fonte a cada 6 h.
- **Senado:**
  - `GET /processo?numdias=N` (N ≤ 30) lista os processos atualizados nos
    últimos N dias.
  - O `alteracao` filtra pelo tipo de atualização (lista em
    `/processo/tipos-atualizacao`).
  - `/processo/{id}` traz `dthUltimaAtualizacao` e
    `ultimaInformacaoAtualizada` (ex.: `ASSUNTO`).

### Apensação e casamento entre Casas (ADR 0006)

- **Senado, `apensadoA`:** a tramitação conjunta vem estruturada em
  `/processo/{id}`, com id, identificação, tipo e datas de apensamento e
  desapensamento. O PL 2338/2023 tem 6 entradas, com o PL 21/2020.

  Isso contradiz o comentário em `senado.go`, segundo o qual ela "não aparece
  de forma estruturada" (o código só olhava `processosRelacionados`). Hoje o
  `AttachedTo` do Senado fica sempre vazio, e daria para preenchê-lo.
- **Senado, `outrosNumeros`:** dá o número da mesma Proposição na outra
  Casa. O PL 2338/2023 aponta para o processo 8802692, com
  `casaIdentificadora` `CD`. `idProcessoCasaInicial` e
  `identificacaoProcessoInicial` dizem qual é o processo da Casa iniciadora.

  Isso deixa mais forte a sugestão de casamento que o Editor confirma, mas
  não substitui a confirmação: proposições antigas têm números diferentes
  em cada Casa.
- **Câmara:**
  - `uriPropAnterior` e `uriPropPosterior` ligam Proposições que viraram
    outra.
  - O histórico de apensação só existe como texto na Tramitação
    ("Desapensação desta da PEC nº 221, de 2019…").
  - `uriPropPrincipal` só é preenchido enquanto a Proposição está apensada.

### Temas e palavras-chave

- **Câmara:**
  - `GET /proposicoes/{id}/temas` traz as áreas temáticas oficiais da Câmara
    (código, nome, relevância). A PEC 221/2019 tem 2, entre elas "Direito
    Constitucional".
  - A lista completa está em `/referencias/proposicoes/codTema`.
  - `/proposicoes/{id}` traz `keywords`, uma indexação em texto livre.
- **Senado:**
  - `classificacoes` traz uma hierarquia de assuntos, por exemplo
    "Jurídico / Direito Civil / Responsabilidade Civil".
  - `documento.indexacao` traz os termos de indexação.
  - `apelido` traz o nome popular, por exemplo "Marco Legal da Inteligência
    Artificial".

Os Temas do Nelisa são escolhidos pela equipe. Os oficiais poderiam
aparecer só como sugestão ao Editor.

### Documentos da Proposição: emendas, pareceres, substitutivos

- **Câmara, `GET /proposicoes/{id}/relacionadas`:** são Proposições
  acessórias. A PEC 221/2019 tem 412: 289 requerimentos, 97 RPD, 13 DTQ,
  4 pareceres do relator (PRL), 2 pareceres, 2 emendas, 1 redação final
  e 1 substitutivo. Cada uma é uma Proposição com `urlInteiroTeor` própria.
- **Senado, `GET /processo/documento?idProcesso=`:** são todos os documentos
  com `urlDocumento`. O PL 2338/2023 tem 284, dos quais 243 são emendas,
  8 relatórios e 2 pareceres.
- **Senado, `GET /processo/emenda?idProcesso=`:** são as emendas com autoria,
  colegiado e decisão. São 244 no PL 2338/2023, por exemplo uma "Acolhida
  Parcialmente" na CTIA.
- **Senado, `GET /processo/relatoria?idProcesso=`:** são os relatores, com
  colegiado e datas de designação e destituição.

Isso serve para sugerir uma Versão nova quando sai substitutivo ou redação
final, em vez de o Editor descobrir pela Tramitação.

### Situação e rito

- **Câmara, `statusProposicao`:**
  - `regime`, por exemplo "Especial (Art. 202 c/c 191, I, RICD)";
  - `apreciacao`;
  - `uriUltimoRelator`;
  - `codSituacao`, cuja lista está em `/referencias/proposicoes/codSituacao`.
- **Senado, `/processo/{id}`:**
  - `tramitando` (Sim/Não);
  - `deliberacao`, por exemplo "Aprovada pelo Plenário" com destino "À
    Câmara dos Deputados";
  - `normaGerada`, a lei resultante;
  - `ordensDoDia`, as sessões em que entrou em pauta;
  - `despachos`.
- **Senado, prazos:** `/processo/prazo` dá os prazos regimentais e
  constitucionais, como os das MPVs.

### Votações

- **Câmara:**
  - `GET /proposicoes/{id}/votacoes` lista as votações. A PEC 221/2019 tem
    123, das quais 10 no Plenário. Cada uma traz descrição e `aprovacao`.
  - `GET /votacoes/{id}/votos` dá o voto de cada deputado. Na votação em
    segundo turno da PEC 221/2019 são 480 votos, com nome e partido.
  - `GET /votacoes/{id}/orientacoes` dá a orientação de cada liderança, com
    15 orientações nessa mesma votação.
- **Senado:**
  - `GET /votacao?idProcesso=` traz os votos nominais. Veio vazio para o
    PL 2338/2023.
  - `/votacaoComissao/materia/{sigla}/{numero}/{ano}` traz as votações nas
    comissões.
  - `/plenario/votacao/orientacaoBancada/{data}` traz a orientação de
    bancada.

Mostrar voto por parlamentar e por partido pede desenho de neutralidade
antes, porque o `CONTEXT.md` veda ranking e destaque que sugira opinião.

### Agenda

- **Câmara:** `GET /eventos` e `GET /eventos/{id}/pauta` dão as sessões e
  reuniões, com as Proposições em pauta.
- **Senado:**
  - `/plenario/agenda/dia/{data}` e `/comissao/agenda/{data}` dão as
    agendas;
  - `/plenario/encontro/{codigo}/pauta` dá a pauta de uma sessão;
  - há iCal em `/plenario/agenda/atual/iCal`.

Isso serviria para mostrar "entra em votação esta semana" nas Proposições
acompanhadas.

**Em uso pelo Radar (#45), só no Plenário:**
- **Câmara:** `/eventos?idOrgao=180` (o Plenário) e a pauta de cada sessão.
  O PL também aparece como `proposicaoRelacionada_` de um parecer.
- **Senado:** `/plenario/agenda/dia/{AAAAMMDD}.json`. A pauta traz o
  `CodigoMateria` legado, e `/processo?codigoMateria=` dá o id do processo.
- **Apresentadas:** a Câmara usa `dataApresentacaoInicio/Fim` em
  `/proposicoes`; o Senado, `dataInicioApresentacao/dataFimApresentacao` em
  `/processo`, uma consulta por Tipo.
- **Volume medido em 2026-11-10/11:**
  - Plenário: 13 PL/PEC/PLP em 3 dias na Câmara e cerca de 10 no Senado.
  - Apresentadas por semana: 63 na Câmara e cerca de 15 no Senado.
  - Comissões da Câmara: cerca de 300 em 3 dias. Por isso ficam fora, e a
    pauta das comissões do Senado também, porque só sai em PDF.
- O Senado devolve objeto em vez de lista quando há um item só
  (`httpsrc.OneOrMany`).

### Parlamentares, órgãos e normas

- **Câmara:** `/deputados` (com despesas, discursos e frentes), `/orgaos`
  (com membros), `/partidos`, `/blocos`, `/frentes`, `/legislaturas` (Mesa
  e líderes).
- **Senado:** `/senador/*` (mandatos, filiações, comissões, discursos),
  `/composicao/*` (comissões, Mesa, blocos, lideranças), `/comissao/*`.
- **Senado, `/legislacao/*`:** normas jurídicas federais por tipo, número e
  ano ou por URN. Serve para ligar uma Proposição transformada em norma à
  lei publicada.
- **Senado, `/discurso/texto-integral/{codigo}`** e notas taquigráficas
  (`/taquigrafia/*`).

## Pegadinhas já encontradas

### Câmara

- **Sequência repetida:** a `sequencia` da Tramitação se repete em eventos
  diferentes. A PEC 221/2019 tem dois eventos 407 no mesmo minuto. Por isso
  a chave usada é `sequencia-codTipoTramitacao`, mais `#n` se ainda houver
  repetição.
- **Situação por evento:** o `descricaoSituacao` de cada tramitação não é a
  situação depois do evento. A apresentação da PEC 221/2019 vem com
  "Transformado em Norma Jurídica". A situação atual só está em
  `statusProposicao`.
- **Assinaturas em PEC:** a lista de autores traz também assinaturas que não
  contam (`proponente=0`). Usamos só os proponentes.
- **Origem no Senado:** a Proposição que veio do Senado tem como autor
  "Senado Federal - <senador>". É assim que se deduz que a Câmara é a Casa
  revisora.
- **Busca aproximada:** a busca pode trazer resultados aproximados. Só o
  nome exato é aceito.
- **Ementa detalhada:** `ementaDetalhada` costuma vir vazia.

### Senado

- **Identificação mantida:** desde 2019 o Senado mantém a identificação da
  Casa iniciadora, então "PEC 221/2019" é igual nas duas. Antes disso o
  número pode mudar de uma Casa para a outra.
- **Emenda da Câmara:** um texto que volta da Câmara aparece como
  `PL 282/2024 (Emenda-CD)`, e o `objetivo` vira "Substitutivo" ou
  "Emenda".
- **Sem despacho separado:** o Senado não separa o despacho; o texto do
  informe vai inteiro em `Description`.
- **Host do PDF:** o PDF do texto vem de `legis.senado.gov.br` (`.gov.br`,
  não `.leg.br`).
- **Limite de requisições:** acima de 10 requisições por segundo a API
  responde 429. O adaptador limita a concorrência a 2.

### As duas

- **PDF com carimbos:** o PDF traz cabeçalho, rodapé e carimbos repetidos
  em cada página, como o código `*CD…*` e o "Assinado eletronicamente
  pelo(a)" da Câmara e o código LexEdit `SF/…` do Senado. A limpeza
  (`structuring.Clean`) tira esses trechos; os casos reais ficam em
  `domain/structuring/testdata/`.
- **Resposta hostil:** toda resposta é tratada como hostil, com tamanho
  máximo, campos validados e erro de sub-recurso tratado como inconsistência
  da fonte. Falha de fonte nunca grava dado parcial.
