# 0001 — Identidade própria, sem reaproveitar o nebula

**Status:** aceita (2026-10-28)

## Contexto
Os outros projetos do fogolab (ex.: carol-clima) reaproveitam a identidade do
open-nebula. O Nelisa é público e político: seus usuários (Editores, Revisores,
Admins) são terceiros, não o ecossistema pessoal do autor.

## Decisão
O Nelisa tem login próprio, sem auto-cadastro, com 2FA obrigatório para
Revisor e Admin.

_Atualização 2026-10-28:_ na v1 não há envio de e-mail. Em vez de convite, o
Admin cadastra a pessoa (nome, e-mail, senha inicial, role) e faz o reset de
senha. Senha com no mínimo 8 caracteres, checada contra lista de senhas
vazadas, e **troca obrigatória** no primeiro login e após todo reset — a senha
definida pelo Admin é só temporária, para que a Auditoria e os quatro olhos
continuem provando autoria. 2FA (TOTP) segue obrigatório para Revisor e Admin;
reset de 2FA só pelo Admin, registrado na Auditoria.

## Consequências
- Bases de usuários separadas: comprometer uma não expõe a outra.
- Mais código de auth para manter (convite, senha, 2FA, recuperação) em vez de
  reaproveitar o nebula.
