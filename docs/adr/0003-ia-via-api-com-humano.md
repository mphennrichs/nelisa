# 0003 — IA via API externa, sempre com confirmação humana

**Status:** aceita (2026-10-28)

## Contexto
Escrever Resumos é o gargalo da equipe. Um LLM gera rascunhos bons, mas resumo
enviesado ou alucinado com selo de "revisado" destrói a credibilidade de um
projeto político. Considerou-se rodar um modelo aberto no fogolab.

## Decisão
- IA só gera **Rascunhos** (Resumo e apoio à Estruturação), sob demanda; nunca
  Comentários. Nada vai a "pronto" sem edição/confirmação humana.
- Modelo via API (Claude; v1 pela API da Anthropic direto, possivelmente
  Claude Platform on AWS / Bedrock no lançamento), atrás de uma interface de
  provedor no backend. Limite de gasto mensal configurado na conta.
- Nada de LLM próprio (self-hosted ou fine-tuning).

## Consequências
- Custo estimado de ~US$ 0,10 por Resumo típico (~US$ 10–30/mês).
- Textos de PL são públicos: mandar à API não expõe dado sensível.
- LLM próprio custaria GPU (fogolab) ou centenas de US$/mês (AWS), com
  qualidade inferior em texto jurídico em PT, e não há dados para fine-tuning.
  Reavaliar só com escala muito maior ou acervo de Resumos humanos.
