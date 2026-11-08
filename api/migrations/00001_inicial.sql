-- Migration inicial do Nelisa. Até a ida para a AWS esta é a única
-- migration e pode ser editada: o banco é recriado do zero (ver
-- docs/plano.md). Depois do congelamento, só migrations novas.

-- +goose Up
-- Um schema por módulo (docs/arquitetura.md). Cada módulo acrescenta aqui
-- as próprias tabelas.
CREATE SCHEMA IF NOT EXISTS team;
CREATE SCHEMA IF NOT EXISTS audit;
CREATE SCHEMA IF NOT EXISTS proposition;
CREATE SCHEMA IF NOT EXISTS progress;
-- Tabelas da fila de jobs (River). Quem as cria são as migrations do
-- próprio River, aplicadas pelo modo migrate logo depois desta
-- (platform/migrate); aqui só nasce o schema.
CREATE SCHEMA IF NOT EXISTS river;

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
    -- Desativada pelo Admin: não entra nem renova sessão, mas a conta fica,
    -- porque Auditoria, Autoria e aprovações apontam para ela. NULL = ativa.
    deactivated_at       timestamptz,
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

-- proposition: a Proposição (ADR 0006) e seus Vínculos de fonte.
CREATE TABLE proposition.propositions (
    id               uuid PRIMARY KEY,
    -- Identificador público e estável, para URL e prévia de link
    -- ("pec-221-2019"). Nasce de Tipo/número/ano, que não mudam depois da
    -- Importação; se colidir, ganha a Casa iniciadora no fim
    -- ("pec-10-2015-senado").
    slug             text NOT NULL,
    kind             text NOT NULL CHECK (kind ~ '^[A-Z]{1,10}$'),
    number           integer NOT NULL CHECK (number > 0),
    year             integer NOT NULL CHECK (year BETWEEN 1800 AND 9999),
    sphere           text NOT NULL CHECK (sphere IN ('federal', 'state', 'municipal')),
    -- Código da Casa iniciadora. Na v1, 'camara' ou 'senado'; estaduais e
    -- municipais terão o código da Assembleia/Câmara Municipal.
    origin_chamber   text NOT NULL CHECK (origin_chamber <> ''),
    -- Dados oficiais, copiados da fonte pela Importação e pela
    -- Sincronização. Nada aqui é produzido pelo Nelisa.
    summary          text NOT NULL DEFAULT '',
    presented_at     timestamptz,
    official_text_url text NOT NULL DEFAULT '',
    authors          text[] NOT NULL DEFAULT '{}',
    status           text NOT NULL DEFAULT '',
    -- Data do evento de Tramitação mais recente por trás de status: a Casa
    -- com o evento mais novo é a que dita a situação atual.
    status_at        timestamptz,
    -- Apensação: a principal, se já estiver no Nelisa. attached_to_name
    -- guarda o nome oficial que a fonte deu, para resolver depois.
    attached_to      uuid REFERENCES proposition.propositions (id) ON DELETE SET NULL,
    attached_to_name text NOT NULL DEFAULT '',
    superseded       boolean NOT NULL DEFAULT false,
    review_state     text NOT NULL CHECK (review_state IN ('draft', 'in_review', 'ready')),
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL,
    CONSTRAINT propositions_not_attached_to_self CHECK (attached_to IS NULL OR attached_to <> id)
);
CREATE UNIQUE INDEX propositions_slug_key ON proposition.propositions (slug);
-- A numeração é de cada Casa: "PEC 10/2015" já existiu na Câmara e no
-- Senado como Proposições diferentes, e toda Assembleia e Câmara Municipal
-- tem o seu "PL 1/2026". Por isso a identidade inclui a Casa iniciadora,
-- e não só (Esfera, Tipo, número, ano).
CREATE UNIQUE INDEX propositions_identity_key ON proposition.propositions (origin_chamber, kind, number, year);
CREATE INDEX propositions_name_idx ON proposition.propositions (kind, number, year);
CREATE INDEX propositions_review_state_idx ON proposition.propositions (review_state, updated_at);
CREATE INDEX propositions_attached_to_idx ON proposition.propositions (attached_to);

-- Vínculo de fonte: a Proposição numa Casa com API. O sistema sugere
-- ('suggested'); só o Editor confirma ('confirmed', com quem e quando).
CREATE TABLE proposition.source_links (
    id              uuid PRIMARY KEY,
    proposition_id  uuid NOT NULL REFERENCES proposition.propositions (id) ON DELETE CASCADE,
    chamber         text NOT NULL CHECK (chamber IN ('camara', 'senado')),
    external_id     text NOT NULL CHECK (external_id <> ''),
    state           text NOT NULL CHECK (state IN ('suggested', 'confirmed')),
    -- Como a Casa chama a Proposição ("PEC 221/2019") e a página pública
    -- dela, para o Editor conferir a sugestão e o Leitor a fonte.
    source_name     text NOT NULL DEFAULT '',
    source_url      text NOT NULL DEFAULT '',
    -- Sem chave estrangeira para team.users, como na Auditoria: a conta
    -- pode ser excluída e o vínculo continua.
    confirmed_by    uuid,
    confirmed_at    timestamptz,
    last_synced_at  timestamptz,
    last_sync_error text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL,
    CONSTRAINT source_links_confirmation CHECK (
        (state = 'confirmed') = (confirmed_by IS NOT NULL AND confirmed_at IS NOT NULL)
    )
);
CREATE UNIQUE INDEX source_links_proposition_external_key ON proposition.source_links (proposition_id, chamber, external_id);
-- Um registro de uma Casa pertence a uma Proposição só (entre os
-- confirmados): um vínculo errado misturaria duas Proposições no site.
CREATE UNIQUE INDEX source_links_confirmed_external_key ON proposition.source_links (chamber, external_id)
    WHERE state = 'confirmed';
-- Um Vínculo de fonte confirmado por Casa (CONTEXT.md).
CREATE UNIQUE INDEX source_links_confirmed_chamber_key ON proposition.source_links (proposition_id, chamber)
    WHERE state = 'confirmed';
CREATE INDEX source_links_external_idx ON proposition.source_links (chamber, external_id);

-- progress: a Tramitação, um evento por linha, de todas as Casas.
CREATE TABLE progress.entries (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    proposition_id uuid NOT NULL REFERENCES proposition.propositions (id) ON DELETE CASCADE,
    chamber        text NOT NULL CHECK (chamber <> ''),
    -- Chave estável do evento na fonte: a Sincronização reaplica o mesmo
    -- evento sem duplicar.
    external_key   text NOT NULL CHECK (external_key <> ''),
    at             timestamptz NOT NULL,
    body           text NOT NULL DEFAULT '',
    description    text NOT NULL DEFAULT '',
    situation      text NOT NULL DEFAULT '',
    dispatch       text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL,
    updated_at     timestamptz NOT NULL
);
CREATE UNIQUE INDEX entries_source_key ON progress.entries (proposition_id, chamber, external_key);
CREATE INDEX entries_timeline_idx ON progress.entries (proposition_id, at, chamber, external_key);

-- +goose Down
DROP SCHEMA IF EXISTS river CASCADE;
DROP SCHEMA IF EXISTS progress CASCADE;
DROP SCHEMA IF EXISTS proposition CASCADE;
DROP SCHEMA IF EXISTS audit CASCADE;
DROP SCHEMA IF EXISTS team CASCADE;
