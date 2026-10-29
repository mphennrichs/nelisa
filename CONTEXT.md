# Nelisa

Sistema público de acompanhamento de projetos de lei federais, estaduais e
municipais: cada Proposição é publicada com resumo, link para o documento integral,
documento comentado e tramitação. Tem uma parte pública e uma parte restrita (com login)
onde a equipe cadastra, comenta e revisa.

## Linguagem

**Proposição**:
A unidade central do sistema: uma proposta legislativa acompanhada pelo
Nelisa, com resumo, documento integral (versionado), comentários e tramitação.
Tem um **Tipo** e é sempre chamada pelo nome oficial ("PEC 221/2019",
"PL 1234/2026"). É a Proposição que tem estado de Revisão — não o arquivo.
_Evitar_: PL como termo genérico (PL é só um Tipo), Projeto (ambíguo com
projeto de software), Matéria (termo do Senado), documento (é só uma parte)

**Tipo**:
A espécie da Proposição, como a Casa a nomeia: PL (projeto de lei), PEC
(proposta de emenda à Constituição), PLP (projeto de lei complementar), MPV
(medida provisória), PDL etc. Também vale para estaduais e municipais.
_Evitar_: Categoria, classe

**Apensação**:
Quando a Casa junta uma Proposição a outra (a principal) para tramitarem
juntas. A apensada aponta para a principal, e a principal lista as apensadas.

**Prejudicada**:
Proposição que deixa de tramitar porque outra (em geral a principal a que
estava apensada) foi aprovada ou rejeitada. Vai para o arquivo sem ser votada.

**Casa legislativa**:
O órgão onde a Proposição tramita — Câmara dos Deputados, Senado, uma Assembleia
Legislativa ou uma Câmara Municipal. Uma Proposição tramita por **uma ou mais**
Casas (ex.: PEC iniciada na Câmara e revista no Senado) e continua sendo **um
registro só**, com uma Tramitação única; a primeira é a Casa iniciadora. As
Casas definem a Esfera e o lugar (UF, município). O Nelisa aceita Proposições
de **todas** as Casas do Brasil.
_Evitar_: Órgão, parlamento, legislativo

**Parlamentar**:
Pessoa eleita para uma Casa legislativa, com seus dados públicos e os de seus
**Mandatos**. Na Importação vem com o ID oficial da fonte; no municipal, o
Editor cria ou reaproveita um existente. A página do Parlamentar mostra dados
públicos e a lista de Proposições — sem notas, rankings ou índices (neutralidade).
_Evitar_: Político, deputado/vereador (como termo genérico)

**Mandato**:
Período de um Parlamentar em uma Casa, com partido e UF. Partido é do Mandato,
não da pessoa: a Proposição mostra o partido **na data da apresentação**.

**Autor**:
Quem apresentou a Proposição — um ou mais Parlamentares **ou** órgãos (Executivo,
comissão, iniciativa popular).
_Evitar_: Proponente, autoria (ambíguo com Autoria editorial)

**Voto nominal**:
Como cada Parlamentar votou numa votação da Proposição. **Só existe quando importado de
fonte oficial** (v1: Câmara e Senado); nunca é digitado nem vem de
Contribuição. Sem dado público, o Nelisa não exibe voto algum — só o resultado
agregado da Tramitação. Votação simbólica aparece como tal.

**Esfera**:
O âmbito da Proposição: federal (nacional), estadual ou municipal. Define a prioridade
na Fila de revisão — federal primeiro, municipal por último.
_Evitar_: Nível, escopo, tipo

**Documento integral**:
O texto oficial da Proposição **na Fonte oficial** — o Nelisa guarda e exibe o link,
não serve uma cópia. É **versionado**: substitutivo, emenda aprovada ou nova
redação geram uma nova Versão.

**Versão**:
Uma edição do Documento integral. Nova versão devolve a Proposição para "em revisão";
os Comentários ficam presos à versão em que foram feitos.

**Versão vigente** / **Versão em análise**:
A Versão vigente é a última que chegou a "pronto" — é a que o Leitor vê, com
Resumo e Documento comentado. Enquanto uma Versão nova é revisada (Versão em
análise), a parte pública avisa que há texto novo em análise e aponta o link
oficial dele. Comentários de Dispositivos inalterados são trazidos como "a
confirmar"; o Resumo também precisa ser confirmado ou reescrito.

**Snapshot** _(fora da v1)_:
Cópia interna do arquivo da Fonte oficial de uma Versão, com data e hash.
Serve à Estruturação, à detecção de Versão nova e como evidência se a fonte
sumir. **Nunca** é servida ao público.

**Verificação de fonte** _(fora da v1)_:
Checagem do link oficial de cada Proposição **duas vezes por semana**: se o link caiu,
a Proposição ganha o aviso "link oficial indisponível" e entra na Fila de revisão; se o
texto estruturado mudou, gera uma Mudança detectada.

**Mudança detectada** _(fora da v1 para Proposições sem API)_:
Diferença, Dispositivo por Dispositivo, entre o texto atual da fonte e a Versão
vigente. Não cria Versão sozinha: entra na Fila de revisão e um Editor confirma
(vira nova Versão) ou descarta (erro de extração, página quebrada). Mudanças só
no arquivo (metadado, layout) apenas atualizam o Snapshot.

**Dispositivo**:
Unidade estrutural do texto legal (LC 95/98): artigo, parágrafo, inciso,
alínea, item — mais ementa e justificativa. Tem endereço estável (ex.:
"Art. 3º, § 2º, IV") dentro de uma Versão.
_Evitar_: Linha, seção, trecho (como unidade de âncora)

**Estruturação**:
A conversão do arquivo original (PDF/DOCX/HTML) de uma Versão em Dispositivos.
É automática, mas o Editor pode corrigi-la antes de comentar. Em Proposições sem API,
o Editor fornece o texto (arquivo ou colado) — o Nelisa só busca sozinho em
fontes com API cadastrada.

**Comentário**:
Análise **pública** de um Editor ou Revisor presa a um Dispositivo de uma
Versão — opcionalmente a um trecho de texto dentro dele. Só aparece para o
Leitor quando a Versão chega a "pronto"; antes disso é rascunho.
_Evitar_: Nota, anotação solta

**Nota interna**:
Conversa da equipe presa a uma Proposição ou a um Dispositivo, em thread. **Nunca**
aparece para o Leitor. Qualquer pessoa da equipe escreve, com ou sem
Posse.
_Evitar_: Comentário interno, recado

**Documento comentado**:
A cópia do Nelisa: o texto de uma Versão, já estruturado em Dispositivos, com
seus Comentários. Visto na página da Proposição (sem download em PDF na v1).

**Resumo**:
Explicação em linguagem acessível do que a Proposição propõe, escrita por Editor ou
Revisor. Pode partir de um **Rascunho de IA**, mas nunca vai a "pronto" sem
edição ou confirmação humana explícita.

**Resumo curto**:
Versão de até ~280 caracteres do Resumo, escrita pela equipe e aprovada junto
com ele (quatro olhos). É o texto da Imagem de compartilhamento e da prévia do
link. Nunca é um corte automático do Resumo.

**Tema**:
Classificação editorial da Proposição (1 a 3 por Proposição) a partir de uma lista fechada
mantida pelo Admin — ex.: saúde, educação, transporte. Escolhido pela equipe
(o Rascunho de IA sugere) e aprovado junto com o Resumo. Por ser produção do
Nelisa, só aparece ao Leitor em Proposições prontas.
_Evitar_: Tag, categoria, assunto

**Rascunho de IA**:
Texto gerado sob demanda por um Editor para servir de ponto de partida do
Resumo, do Resumo curto, dos Temas ou da Estruturação. Nunca é usado em Comentários. Seu uso fica na Auditoria; o
público é informado por uma política geral, não por selo em cada Proposição.

**Revisão** (estado editorial da Proposição):
O ciclo de trabalho do Nelisa sobre uma Proposição: **criado** → **em revisão** →
**pronto**. "Criado" não aparece para o público; "em revisão" aparece como
**em análise** — o Leitor vê tudo que **não é produzido pelo Nelisa**
(identificação, ementa oficial, autor, Tramitação, link do Documento integral),
mas nada do que é (Resumo, Comentários, Temas); "pronto" aparece completo. Só o Revisor marca como pronto.
_Evitar_: Publicado (confunde com a publicação oficial na Tramitação)

**Correção menor**:
Ajuste em conteúdo do Nelisa de uma Proposição pronta que não muda o sentido (digitação,
formatação, link). O Editor aplica direto, a Proposição segue pronta, fica na Auditoria
e entra na Fila como "correção a conferir" para um Revisor olhar depois.

**Errata**:
Correção que muda o sentido de um Resumo ou Comentário de uma Proposição pronta. Exige
aprovação de outro Revisor (quatro olhos); enquanto isso a Proposição segue pronta com
o conteúdo antigo. Aprovada, aparece ao Leitor como nota datada ("dizia X; o
correto é Y"). Diferente de nova Versão: aqui quem errou foi o Nelisa, não o
texto da Proposição que mudou.

**Tramitação**:
O percurso legislativo real da Proposição: autor, linha do tempo (apresentação,
publicação oficial, votação), resultado da votação e entrada em vigor.
Mudanças na Tramitação **não** alteram o estado de Revisão.
_Evitar_: Status, andamento (quando se referir à Revisão)

**Importação**:
Trazer uma Proposição de uma API pública (v1: Câmara e Senado) **sob demanda**: o
Editor busca e escolhe a Proposição, que entra como "criado" com Documento integral e
Tramitação preenchidos. Nunca é em massa. Indicações de links dessas fontes
viram Importação.
_Evitar_: Crawler, raspagem

**Sincronização**:
Atualização periódica das Proposições importadas a partir da fonte: Tramitação nova é
aplicada direto; texto novo gera uma Mudança detectada.
Proposições sem API não sincronizam — dependem do Editor e de Contribuições.

**Fonte oficial**:
Domínio reconhecido como pertencente a uma Casa legislativa (ou órgão
oficial ligado a ela). `*.leg.br` e `*.gov.br` são aceitos por padrão; outros são cadastrados
por Editor ou Revisor e removidos pelo Admin.

**Fila de revisão**:
A lista de trabalho da equipe: Proposições a revisar, Mudanças detectadas e
Contribuições pendentes, ordenados **sempre** por Esfera (federal > estadual >
municipal) — um item municipal só sobe quando não há nada nas esferas acima.
Dentro da mesma Esfera, vem primeiro a Proposição com votação mais próxima; itens sem
votação marcada vêm depois, do mais antigo na fila para o mais novo. A equipe
pode filtrar a Fila por Esfera.

**Posse**:
Um Editor ou Revisor **assume** um item da Fila; enquanto tiver a posse, só
ele edita Resumo e Comentários daquela Proposição (os outros leem). A posse expira após
**3 dias sem atividade**, e Revisor ou Admin podem liberá-la. Aprovar (marcar
pronto) não exige posse. O nome de quem tem a posse é visível só para a equipe.
_Evitar_: Lock, trava, reserva

**Compartilhamento**:
Toda Proposição tem link permanente compartilhável (com prévia nas redes) e pode ser
compartilhado como **Imagem de compartilhamento**: o Resumo curto e a URL da
página da Proposição sobre fundo em gradiente, no formato story (vertical 9:16). O
Leitor escolhe o gradiente numa paleta curada **sem associação partidária**.
Só Proposições prontas geram imagem.
Não existe "acompanhar" Proposição (sem
cadastro de Leitor, sem e-mail, sem RSS) na v1.

### Pessoas

**Leitor**:
Qualquer pessoa sem login. Lê a parte pública, compartilha e envia Contribuições.

**Editor**:
Sobe documentos, escreve Resumos, faz Comentários, mantém Tramitação manual e
avalia Contribuições.

**Revisor**:
Tudo que o Editor faz, e também marca uma Proposição como pronta — mas **nunca** uma Proposição
cujo Resumo ele mesmo escreveu (quatro olhos).

**Autoria editorial**:
Quem escreveu o Resumo e os Comentários e quem aprovou (marcou pronto) cada
Versão. Registrada internamente; **não** é exibida ao público — o Leitor não vê
nomes da equipe.
_Evitar_: Lista de revisores (requisito removido)

**Admin**:
Acesso a tudo e a todos; gerencia usuários, roles e Fontes oficiais. É o único
que pode aprovar o próprio trabalho (furar os quatro olhos).

**Cadastro de equipe**:
A única forma de entrar na equipe: o Admin cadastra a pessoa (nome, e-mail,
senha inicial, role). Não existe auto-cadastro nem convite por e-mail na v1;
reset de senha também é feito pelo Admin. Contas são desativadas, nunca
apagadas — a Autoria editorial das Proposições que a pessoa trabalhou continua íntegra.
_Evitar_: Convite (não existe na v1), registro

**Selo de revisão**:
O que o Leitor vê sobre a revisão de uma Proposição, sem nomes: "revisado por 2 pessoas"
ou, quando o Admin aprovou o próprio trabalho, "revisado por 1 pessoa".

**Segunda revisão pendente**:
Marca na Fila de revisão de uma Proposição aprovada pelo Admin sem quatro olhos. A Proposição
segue pronta; quando outro Revisor confirma, o Selo passa a "2 pessoas".

**Auditoria**:
Registro interno, obrigatório e inapagável (nem pelo Admin) de toda aprovação
(quem, quando, qual Versão, se houve furo) e de toda ação sensível do Admin
(convidar, promover, rebaixar, desativar, remover Fonte oficial).
_Evitar_: Log (é técnico; Auditoria é do domínio)

### Contribuições

**Contribuição**:
Algo enviado por um Leitor sem login. Tem estado próprio — **pendente** →
**aceita** ou **rejeitada** — e nunca altera a Proposição enquanto pendente. Exige
fonte (link ou anexo) e passa por captcha e rate limit. O Leitor recebe um
**código de acompanhamento** para consultar o status numa página pública; o
Nelisa não coleta e-mail nem dado pessoal (o IP serve só ao rate limit e some
em 24h). Avaliada por Editor ou Revisor.
_Evitar_: Sugestão, envio, ticket

**Indicação**:
Contribuição que aponta o endereço de uma Proposição nova. Na v1 o Nelisa **não busca**
o endereço: recusa na hora só se o domínio não for Fonte oficial; o resto (o
link existe, é mesmo uma Proposição, já está cadastrada) o Editor confere à mão. Se o
endereço é de uma fonte com API (Câmara, Senado), o Editor faz a Importação.
Quem cria a Proposição a partir da Indicação é o Editor.

**Atualização de tramitação**:
Contribuição estruturada (tipo de evento, data, fonte) sobre a Tramitação de um
Proposição existente — ex.: votação adiada.

**Pedido de fonte oficial**:
Contribuição pedindo que um domínio seja cadastrado como Fonte oficial, com
evidência de que pertence à casa legislativa ou órgão.
