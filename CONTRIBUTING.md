# Como contribuir

Obrigado pelo interesse no Nelisa. O projeto explica Proposições legislativas
com revisão humana; contribuições de código, de texto e de dados são bem-vindas.

## Por onde começar

- O andamento está no [quadro Nelisa v1](https://github.com/users/mphennrichs/projects/1)
  e nos [milestones](https://github.com/mphennrichs/nelisa/milestones), em
  fases (ver [`docs/plano.md`](docs/plano.md)).
- Issues com `good first issue` são boas para começar.
- Antes de começar, comente na issue que vai pegá-la, para ninguém fazer o
  mesmo trabalho em paralelo.
- Ideia nova ou bug: abra uma issue antes do PR.

## Leitura obrigatória

1. [`CONTEXT.md`](CONTEXT.md): o vocabulário do domínio.
2. [`docs/arquitetura.md`](docs/arquitetura.md): camadas, padrões e regras.
3. [`docs/nomes-no-codigo.md`](docs/nomes-no-codigo.md): domínio em português,
   código em inglês.
4. Os [ADRs](docs/adr/) da área em que você vai mexer.

## Usando agentes de IA

Pode usar. O [`AGENTS.md`](AGENTS.md) traz as regras que o agente precisa
seguir (o Claude Code lê pelo `CLAUDE.md`; outros agentes leem o `AGENTS.md`
direto). Você continua responsável pelo PR: revise o código, rode os testes e
não envie nada que você não entenda.

## Rodando localmente

Veja a seção "Estado atual" do [README](README.md): Postgres de
desenvolvimento, API em Go e o `web/` apontando para ela. A demonstração
fica em `/demo` e a área real da equipe em `/equipe`.

## Pull requests

- Um assunto por PR, com `Closes #<issue>` na descrição.
- Testes passando localmente antes do push. O CI precisa estar verde para o
  merge.
- Siga o checklist do template de PR.
- Textos de interface em português; código, nomes e testes em inglês.
- Só o mantenedor faz merge na `main`; tudo que entra na `main` vai para o
  ambiente do projeto.

## Conduta

Projeto político, discussão técnica. Critique ideias e código, nunca pessoas.
Debate partidário não cabe nas issues: o Nelisa explica Proposições, não toma
lado.

## Segurança

Achou uma vulnerabilidade? Não abra issue pública. Use o
[aviso privado de segurança do GitHub](https://github.com/mphennrichs/nelisa/security/advisories/new).
