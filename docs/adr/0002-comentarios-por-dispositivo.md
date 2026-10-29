# 0002 — Comentários ancorados em Dispositivos, não em linhas

**Status:** aceita (2026-10-28)

## Contexto
O Documento integral chega como PDF, DOCX ou HTML. "Linha" depende da
diagramação e muda entre Versões, então comentários presos a linhas quebram.

## Decisão
Cada Versão passa por Estruturação (texto → Dispositivos, segundo a LC 95/98).
Comentários se prendem a um Dispositivo (e, opcionalmente, a um trecho dele).
Ementa e justificativa também são âncoras.

## Consequências
- Endereços estáveis permitem comparar Versões por Dispositivo.
- Exige um parser de texto legislativo e uma tela para o Editor corrigir a
  Estruturação — PLs municipais mal formatados vão errar.
- Não dá para citar "linha N do PDF original".
