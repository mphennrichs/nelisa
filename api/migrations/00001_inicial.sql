-- Migration inicial do Nelisa. Até a ida para a AWS esta é a única
-- migration e pode ser editada: o banco é recriado do zero (ver
-- docs/plano.md). Depois do congelamento, só migrations novas.

-- +goose Up
-- Um schema por módulo (docs/arquitetura.md). Cada módulo acrescenta aqui
-- as próprias tabelas.
CREATE SCHEMA IF NOT EXISTS team;
CREATE SCHEMA IF NOT EXISTS audit;

-- team: cadastro da equipe e sessões (ADR 0001). Sem auto-cadastro: o
-- primeiro Admin nasce pelo bootstrap e os demais são cadastrados por ele.
CREATE TABLE team.users (
    id                   uuid PRIMARY KEY,
    name                 text NOT NULL,
    email                text NOT NULL,
    password_hash        text NOT NULL,
    role                 text NOT NULL CHECK (role IN ('editor', 'reviewer', 'admin')),
    -- Senha definida pelo Admin (cadastro ou reset) é temporária. A troca
    -- obrigatória ainda não é aplicada (issue #11); a coluna já existe.
    must_change_password boolean NOT NULL DEFAULT false,
    -- Tokens emitidos antes deste instante são recusados: troca ou reset de
    -- senha encerra todas as sessões. NULL = sem corte.
    sessions_valid_after timestamptz,
    created_at           timestamptz NOT NULL
);
-- E-mail único sem diferenciar maiúsculas: "Ana@x.org" e "ana@x.org" são a
-- mesma pessoa.
CREATE UNIQUE INDEX users_email_key ON team.users (lower(email));

-- Refresh token opaco: só o hash (SHA-256) fica no banco. Cada uso o gira
-- (revoga o apresentado e emite outro); um token gasto que volta depois da
-- carência denuncia cópia e encerra todas as sessões da pessoa.
CREATE TABLE team.refresh_tokens (
    id         uuid PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES team.users (id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz
);
CREATE INDEX refresh_tokens_user_id_idx ON team.refresh_tokens (user_id);

-- audit: Auditoria. Só cresce; sem chave estrangeira para team.users para
-- que o histórico sobreviva à exclusão de quem agiu ou foi alvo.
CREATE TABLE audit.events (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- NULL quando a ação é do sistema (job), não de uma pessoa.
    actor_id    uuid,
    action      text NOT NULL,
    target_type text NOT NULL,
    target_id   text NOT NULL,
    at          timestamptz NOT NULL,
    metadata    jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX events_target_idx ON audit.events (target_type, target_id, at);
CREATE INDEX events_actor_idx ON audit.events (actor_id, at);

-- +goose Down
DROP SCHEMA IF EXISTS audit CASCADE;
DROP SCHEMA IF EXISTS team CASCADE;
