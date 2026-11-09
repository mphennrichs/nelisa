# Fase 3: Contribuições (#24–#28)

Termos do [`CONTEXT.md`](../../CONTEXT.md) ("Contribuição", "Indicação",
"Atualização de tramitação", "Pedido de fonte oficial", "Fonte oficial",
"Fila de revisão"); nomes de [`nomes-no-codigo.md`](../nomes-no-codigo.md)
(`Contribution`, `TrackingCode`, `OfficialSource`). Módulo novo
`api/internal/contribution`.

**Pronto quando:** um Leitor anônimo manda uma Atualização de tramitação da
PEC 221/2019 com link de fonte, passa pelo Turnstile, recebe o código de
acompanhamento, e um Editor aceita a Contribuição pela Fila. O Leitor vê
"aceita" ao consultar o código e o evento aparece na Tramitação pública.

## Fora deste desenho

- **Anexo.** O `CONTEXT.md` diz "link ou anexo", mas na v1 vai só link.
  Upload traz armazenamento, tipo de arquivo, malware e dado pessoal em PDF
  ou foto, e precisa de desenho próprio. Vira issue Pós-v1, e o `CONTEXT.md`
  passa a dizer "fonte (link; anexo depois da v1)". Ver as Decisões.
- **Buscar o link do Leitor.** O Nelisa nunca abre, baixa nem pré-visualiza
  o que o Leitor manda (AGENTS.md, "nada de URL arbitrária").
- Login de Leitor (#31), notificação por e-mail e qualquer contato com o
  Leitor.

## 1. Formulário (#24)

Página pública `/contribuir`, também acessível pelo botão "Contribuir" na
página da Proposição, que já preenche a Proposição. São três tipos:

| Tipo | Campos | Checagem na hora |
|---|---|---|
| Indicação | link da Proposição; Esfera; observação opcional | o domínio tem de ser Fonte oficial |
| Atualização de tramitação | Proposição (já escolhida); tipo de evento (lista fechada: votação marcada, votação adiada, aprovada, rejeitada, retirada de pauta, outro); data; link de fonte; observação opcional | a Proposição existe e é pública; a data não passa de 1 ano no futuro |
| Pedido de fonte oficial | domínio; link de evidência; nome da Casa ou órgão; Esfera | o domínio é válido e ainda não é Fonte oficial |

Limites:
- Os links são `https`, com até 2.000 caracteres, sem usuário ou senha e com
  host DNS válido (IP literal é recusado). Fora isso, o Nelisa não os toca.
- Texto livre tem até 1.000 caracteres. Junto do campo vai o aviso "não
  escreva dados pessoais".
- Nada é coletado além do que está no formulário: nem e-mail nem nome.

Ao enviar, o Leitor vê o **código de acompanhamento**, num formato como
`K7Q4-9MZD-2HXP`, com botão de copiar e o aviso de que o código não pode
ser recuperado.

## 2. Código de acompanhamento

- **Formato:** 12 caracteres em base32 Crockford (60 bits, sem I/L/O/U),
  vindos de `crypto/rand`. A leitura aceita minúscula, sem hífen, e lê O
  como 0 e I/L como 1.
- **Guarda:** o banco guarda só o `sha256` do código. Quem tiver acesso ao
  banco não consegue consultar como o Leitor.
- **Consulta:** página `/acompanhar` com campo de código. A consulta é
  `POST /api/public/contributions/lookup` com o código no corpo, não na URL,
  para não ficar em log nem em referer. Tem rate limit próprio de 30/h por
  IP.
- **O que a consulta mostra:**
  - tipo, data de envio e estado (pendente, aceita ou rejeitada);
  - se rejeitada, o motivo público;
  - se aceita, o link para a Proposição.

  Nunca mostra o texto enviado nem quem moderou.

## 3. Fonte oficial

- **Padrão:** domínios terminados em `.leg.br` ou `.gov.br`, no código.
- **Cadastro:** a tabela `contribution.official_sources` guarda domínio,
  nome da Casa ou órgão, Esfera, quem cadastrou e quando.
  - Cadastram: Editor e Revisor.
  - Remove: só o Admin.
  - Toda mudança é auditada.
- **Telas:** `/equipe/fontes-oficiais` com lista, cadastro e remoção.
- **Casamento de domínio:** o host do link tem de ser o domínio cadastrado
  ou um subdomínio dele, comparado em minúsculas e sem ponto final.
- **Aceitar um Pedido de fonte oficial** cadastra o domínio na mesma
  transação.
- **Separado da Estruturação:** a lista fechada de hosts da Estruturação
  (busca automática) é outra coisa e não muda. Fonte oficial só serve para
  aceitar ou recusar o link de uma Indicação.

## 4. Moderação pela Fila (#25)

- Contribuições pendentes entram na Fila de revisão.
- **Ordem na Fila:**
  - A Atualização de tramitação herda a Esfera da Proposição.
  - A Indicação e o Pedido de fonte oficial usam a Esfera que o Leitor
    escolheu, e a equipe pode corrigir.
  - Dentro da Esfera vale a regra de sempre: votação mais próxima primeiro,
    depois as mais antigas na Fila.
- **Quem modera:** Editor ou Revisor. Não há quatro olhos aqui, porque a
  Contribuição não publica conteúdo editorial: a Tramitação não altera a
  Revisão (`CONTEXT.md`). Cada decisão é auditada (`contribution.accept` e
  `contribution.reject`).
- **Rejeitar:** pede um motivo público de uma lista fechada:
  - fonte não confirma;
  - fora do escopo;
  - já cadastrada;
  - duplicada;
  - informação incorreta;
  - outro.

  Junto vai uma nota interna opcional, que nunca é pública.
- **Aceitar, conforme o tipo:**
  - **Indicação:** marca como aceita e leva o Editor para a Importação, ou
    para "nova Proposição" com o link já preenchido. Quem cria a Proposição
    continua sendo o Editor (`CONTEXT.md`); a Contribuição guarda o id da
    Proposição criada, e a consulta passa a mostrar o link.
  - **Atualização de tramitação:**
    - O Editor revisa o texto do evento antes de confirmar e grava um
      evento de Tramitação **manual**: `chamber` da Casa onde ocorreu,
      `external_key = contribution:<id>`, link de fonte guardado e
      indicador "manual".
    - O público vê o evento na linha do tempo com o link da fonte.
    - Quando a Sincronização trouxer o evento oficial, os dois convivem.
      O Editor pode apagar o manual depois, ação auditada.
  - **Pedido de fonte oficial:** cadastra a Fonte oficial (seção 3).
- **Texto do Leitor na tela da equipe:**
  - Aparece como texto puro.
  - Os links aparecem com o domínio em destaque e
    `rel="noopener noreferrer nofollow"`.
  - O Nelisa não gera pré-visualização.

## 5. Anti-abuso (#26, #27)

- **Turnstile:**
  - O widget fica no formulário e o token é validado no Go com o
    `siteverify` da Cloudflare antes de qualquer gravação. A validação
    confere `success`, `hostname` (de `PUBLIC_BASE_URL`) e `action`
    (`contribute`).
  - Os segredos vêm de env: `TURNSTILE_SITE_KEY` (público) e
    `TURNSTILE_SECRET`.
  - Em dev e nos testes usamos as chaves de teste públicas da Cloudflare.
  - Se o siteverify estiver fora do ar, a Contribuição é recusada com
    "tente de novo", nunca aceita sem checagem.
- **IP do Leitor:**
  - O IP vem de `CF-Connecting-IP` **só** se a conexão chegou de um proxy
    confiável (`platform/clientip`, o mesmo do limite do login). A lista
    vem de `TRUSTED_PROXY_CIDRS`; sem ela, valem as redes privadas e o
    loopback, que cobrem o Next e o `cloudflared` no stack. Fora disso,
    vale o `RemoteAddr`.
- **Rate limit:**
  - 5 Contribuições por hora por IP (`CONTRIBUTION_RATE_PER_HOUR`).
  - O contador fica no Postgres, para valer com mais de uma instância da
    API. A chave é o `HMAC-SHA256(IP, chave do dia)`.
  - A chave do dia é aleatória, guardada no banco e trocada a cada 24 h.
    Um job River apaga contadores e chaves com mais de 24 h. O IP em claro
    nunca é gravado nem logado (`CONTEXT.md`: "some em 24h").
- **Teto da Fila:**
  - Com 200 Contribuições pendentes ou mais (`CONTRIBUTION_PENDING_CAP`),
    o formulário pausa.
  - `GET /api/public/contributions/availability` diz se está aberto, e a
    página mostra "Contribuições pausadas: a equipe está em dia com as
    pendentes e reabre logo".
  - A checagem é refeita no `POST`.
- **Liga e desliga:**
  - O formulário só existe com `CONTRIBUTIONS_ENABLED=true`. Os passos 1 e
    2 vão para produção desligados e só ligamos depois do passo 3, com o
    Turnstile e o rate limit no ar.

## 6. Dados guardados

`contribution.contributions` guarda:
- id, tipo, estado;
- `tracking_code_hash` (único);
- `proposition_id` (opcional), `sphere`;
- `payload` (jsonb validado por tipo);
- `submitted_at`;
- na moderação: `decided_by`, `decided_at`, `public_reason`, `internal_note`;
- `created_proposition_id`.

Nada do Leitor além do formulário.

## Passos

1. **Envio** (feito, #24, PR #40): módulo `contribution`, Fonte oficial,
   formulário, código e consulta, tudo atrás de `CONTRIBUTIONS_ENABLED`.
   No ar desligado desde 2026-11-09. Enquanto o passo 3 não chega, o envio
   tem um limite provisório em memória (5/h por IP) e a consulta, 30/h.
   A Atualização de tramitação só parte da página da Proposição (o Leitor
   não digita o slug).
2. **Moderação:** Contribuições na Fila, aceitar e rejeitar, Tramitação
   manual. Deploy desligado.
3. **Anti-abuso:** Turnstile, IP confiável, rate limit, teto. Deploy e
   ligamos.
4. **Beta (#28):** checklist da issue.

## Decisões (confirmadas em 2026-11-09)

- Anexo fica para depois da v1, e a v1 aceita só link.
- Código guardado só como hash; consulta por `POST`.
- Moderação sem quatro olhos (não publica conteúdo editorial), mas
  auditada.
- Aceitar uma Atualização de tramitação grava evento manual na Tramitação,
  com link de fonte, revisado pelo Editor antes de gravar.
- Fonte oficial só serve para checar a Indicação; a lista de busca
  automática da Estruturação continua fechada no código.
- Rate limit e teto no Postgres, com IP só como HMAC com chave diária.
- Formulário vai para produção desligado até o anti-abuso estar no ar.
