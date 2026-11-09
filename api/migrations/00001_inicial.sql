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
CREATE SCHEMA IF NOT EXISTS review;
CREATE SCHEMA IF NOT EXISTS queue;
CREATE SCHEMA IF NOT EXISTS contribution;
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
    official_summary text NOT NULL DEFAULT '',
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

-- Versão: uma edição do Documento integral. A Importação cria a 1
-- ('pending', "Texto original"); o Editor cria as seguintes à mão. A
-- aprovação leva a 'pending' a 'current' e a vigente anterior a
-- 'superseded'.
CREATE TABLE proposition.versions (
    id                uuid PRIMARY KEY,
    proposition_id    uuid NOT NULL REFERENCES proposition.propositions (id) ON DELETE CASCADE,
    seq               integer NOT NULL CHECK (seq > 0),
    label             text NOT NULL CHECK (label <> '' AND char_length(label) <= 120),
    -- Vazio só quando a fonte não deu o link na Importação.
    official_text_url text NOT NULL DEFAULT '',
    state             text NOT NULL CHECK (state IN ('pending', 'current', 'superseded')),
    -- NULL quando veio da Importação. Sem chave estrangeira para
    -- team.users, como na Auditoria.
    created_by        uuid,
    created_at        timestamptz NOT NULL
);
CREATE UNIQUE INDEX versions_seq_key ON proposition.versions (proposition_id, seq);
-- No máximo uma em análise e uma vigente por Proposição: é o banco que
-- garante, mesmo com duas aprovações ou criações concorrentes.
CREATE UNIQUE INDEX versions_one_pending_key ON proposition.versions (proposition_id) WHERE state = 'pending';
CREATE UNIQUE INDEX versions_one_current_key ON proposition.versions (proposition_id) WHERE state = 'current';

-- Tema: lista fechada mantida pelo Admin. Desativado some da escolha, mas
-- continua nas Versões que já o usam (por isso nunca é apagado).
CREATE TABLE proposition.topics (
    id         uuid PRIMARY KEY,
    name       text NOT NULL CHECK (name <> '' AND char_length(name) <= 60),
    active     boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX topics_name_key ON proposition.topics (lower(name));

-- Conteúdo editorial de uma Versão (1:1): Resumo, Resumo curto e Temas.
-- Nada daqui sai para o Leitor enquanto a Versão não for a vigente.
CREATE TABLE proposition.version_contents (
    version_id         uuid PRIMARY KEY REFERENCES proposition.versions (id) ON DELETE CASCADE,
    summary            text NOT NULL DEFAULT '' CHECK (char_length(summary) <= 5000),
    short_summary      text NOT NULL DEFAULT '' CHECK (char_length(short_summary) <= 280),
    -- Sem chave estrangeira em array; o caso de uso confere que existem.
    topic_ids          uuid[] NOT NULL DEFAULT '{}' CHECK (cardinality(topic_ids) <= 3),
    -- Copiado da Versão anterior: precisa ser confirmado ou reescrito.
    needs_confirmation boolean NOT NULL DEFAULT false,
    -- Autoria editorial: todo mundo que já gravou ou confirmou este
    -- conteúdo. É a base dos quatro olhos.
    authors            uuid[] NOT NULL DEFAULT '{}',
    -- Trava otimista: sobe a cada gravação; quem grava com um valor velho
    -- leva 409 em vez de apagar o trabalho de outra pessoa.
    revision           integer NOT NULL DEFAULT 0 CHECK (revision >= 0),
    updated_by         uuid,
    updated_at         timestamptz NOT NULL
);

-- Estruturação de uma Versão (1:1): o texto que gerou os Dispositivos.
-- O Nelisa busca o PDF oficial sozinho só em Fonte oficial com API; o
-- PDF não é guardado. source_text é interno: nunca sai no público.
CREATE TABLE proposition.structurings (
    version_id   uuid PRIMARY KEY REFERENCES proposition.versions (id) ON DELETE CASCADE,
    state        text NOT NULL CHECK (state IN ('fetching', 'ready_to_check', 'confirmed', 'manual')),
    failure      text NOT NULL DEFAULT '' CHECK (failure IN (
        '', 'no_url', 'host_not_allowed', 'not_found', 'not_pdf', 'too_large', 'no_text', 'network')),
    source_text  text NOT NULL DEFAULT '' CHECK (char_length(source_text) <= 500000),
    -- Avisos do parser para o Editor: [{code, line, address}].
    warnings     jsonb NOT NULL DEFAULT '[]',
    revision     integer NOT NULL DEFAULT 0 CHECK (revision >= 0),
    -- NULL quando foi a busca automática.
    updated_by   uuid,
    updated_at   timestamptz NOT NULL,
    confirmed_by uuid,
    confirmed_at timestamptz,
    -- manual sempre diz o motivo; os outros estados nunca têm motivo.
    CHECK ((state = 'manual') = (failure <> ''))
);

-- Dispositivos de uma Versão, gerados pelo parser a partir do source_text
-- (nunca editados um a um). address é a âncora estável dos Comentários.
CREATE TABLE proposition.provisions (
    version_id     uuid NOT NULL REFERENCES proposition.structurings (version_id) ON DELETE CASCADE,
    address        text NOT NULL CHECK (address <> ''),
    parent_address text NOT NULL DEFAULT '',
    kind           text NOT NULL CHECK (kind IN (
        'ementa', 'preamble', 'heading', 'article', 'paragraph', 'item', 'point', 'subitem', 'justification')),
    label          text NOT NULL,
    text           text NOT NULL,
    depth          integer NOT NULL CHECK (depth >= 0),
    position       integer NOT NULL CHECK (position >= 0),
    text_hash      text NOT NULL,
    PRIMARY KEY (version_id, address)
);
CREATE UNIQUE INDEX provisions_position_key ON proposition.provisions (version_id, position);

-- Comentário: análise pública presa a um Dispositivo de uma Versão. Sem
-- chave estrangeira para provisions: reprocessar troca os Dispositivos, e
-- o caso de uso garante que nenhum endereço comentado some. Apagar é
-- soft (deleted_at): fica no banco, some da equipe e do público.
CREATE TABLE proposition.comments (
    id                 uuid PRIMARY KEY,
    version_id         uuid NOT NULL REFERENCES proposition.structurings (version_id) ON DELETE CASCADE,
    provision_address  text NOT NULL CHECK (provision_address <> ''),
    body               text NOT NULL CHECK (body <> '' AND char_length(body) <= 3000),
    -- Trecho dentro do Dispositivo (fora da tela da v1; ver o desenho).
    quote_start        integer,
    quote_end          integer,
    needs_confirmation boolean NOT NULL DEFAULT false,
    -- Comentário da Versão anterior de onde este veio (a confirmar).
    copied_from        uuid,
    authors            uuid[] NOT NULL DEFAULT '{}',
    revision           integer NOT NULL DEFAULT 0 CHECK (revision >= 0),
    created_by         uuid NOT NULL,
    created_at         timestamptz NOT NULL,
    updated_by         uuid,
    updated_at         timestamptz NOT NULL,
    deleted_at         timestamptz,
    CHECK ((quote_start IS NULL) = (quote_end IS NULL) AND (quote_start IS NULL OR quote_start < quote_end))
);
CREATE INDEX comments_version_idx ON proposition.comments (version_id, provision_address, created_at);
CREATE UNIQUE INDEX comments_copied_key ON proposition.comments (version_id, copied_from) WHERE copied_from IS NOT NULL;

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

-- review: aprovações (marcar pronto) de Versões. Uma por Versão.
-- four_eyes = false é o Admin aprovando o próprio trabalho: fica a
-- Segunda revisão pendente até outro Revisor confirmar.
CREATE TABLE review.approvals (
    version_id       uuid PRIMARY KEY REFERENCES proposition.versions (id) ON DELETE CASCADE,
    proposition_id   uuid NOT NULL REFERENCES proposition.propositions (id) ON DELETE CASCADE,
    approved_by      uuid NOT NULL,
    approved_at      timestamptz NOT NULL,
    four_eyes        boolean NOT NULL,
    second_review_by uuid,
    second_review_at timestamptz,
    CONSTRAINT approvals_second_review CHECK (
        (second_review_by IS NULL) = (second_review_at IS NULL)
        AND (NOT four_eyes OR second_review_by IS NULL)
        AND (second_review_by IS NULL OR second_review_by <> approved_by)
    )
);
CREATE INDEX approvals_proposition_idx ON review.approvals (proposition_id, approved_at);
CREATE INDEX approvals_pending_second_idx ON review.approvals (approved_at)
    WHERE NOT four_eyes AND second_review_by IS NULL;

-- review: Correções menores da Versão vigente. Entram direto (o Leitor vê
-- o texto corrigido sem aviso) e ficam "a conferir" até um Revisor que não
-- as fez olhar. before/after guardam o conteúdo inteiro, só para a equipe
-- comparar; nada daqui sai para o Leitor.
CREATE TABLE review.minor_fixes (
    id             uuid PRIMARY KEY,
    proposition_id uuid NOT NULL REFERENCES proposition.propositions (id) ON DELETE CASCADE,
    version_id     uuid NOT NULL REFERENCES proposition.versions (id) ON DELETE CASCADE,
    fields         text[] NOT NULL CHECK (cardinality(fields) > 0),
    before         jsonb NOT NULL,
    after          jsonb NOT NULL,
    created_by     uuid NOT NULL,
    created_at     timestamptz NOT NULL,
    checked_by     uuid,
    checked_at     timestamptz,
    -- Quem corrigiu nunca confere a própria correção.
    CONSTRAINT minor_fixes_checked CHECK (
        (checked_by IS NULL) = (checked_at IS NULL)
        AND (checked_by IS NULL OR checked_by <> created_by)
    )
);
CREATE INDEX minor_fixes_proposition_idx ON review.minor_fixes (proposition_id, created_at);
CREATE INDEX minor_fixes_unchecked_idx ON review.minor_fixes (created_at) WHERE checked_by IS NULL;

-- review: Erratas da Versão vigente. Pendente não muda nada; aprovada
-- (por quem não propôs) troca o conteúdo e sai para o Leitor só como nota
-- datada (said/correct). Recusada e retirada nunca saem.
CREATE TABLE review.errata (
    id             uuid PRIMARY KEY,
    proposition_id uuid NOT NULL REFERENCES proposition.propositions (id) ON DELETE CASCADE,
    version_id     uuid NOT NULL REFERENCES proposition.versions (id) ON DELETE CASCADE,
    -- O conteúdo completo como fica depois da Errata.
    proposed       jsonb NOT NULL,
    said           text NOT NULL CHECK (said <> '' AND char_length(said) <= 500),
    correct        text NOT NULL CHECK (correct <> '' AND char_length(correct) <= 500),
    -- revision do conteúdo vigente quando a Errata foi proposta: se mudou,
    -- a Errata foi escrita sobre um texto que não existe mais.
    base_revision  integer NOT NULL CHECK (base_revision >= 0),
    state          text NOT NULL CHECK (state IN ('pending', 'approved', 'rejected', 'withdrawn')),
    created_by     uuid NOT NULL,
    created_at     timestamptz NOT NULL,
    decided_by     uuid,
    decided_at     timestamptz,
    -- Motivo interno da recusa; nunca sai para o Leitor.
    reason         text NOT NULL DEFAULT '' CHECK (char_length(reason) <= 1000),
    CONSTRAINT errata_decision CHECK (
        (state = 'pending') = (decided_at IS NULL)
        AND (decided_by IS NULL) = (decided_at IS NULL)
        -- Quatro olhos: quem propôs não aprova nem recusa; só retira.
        AND (state NOT IN ('approved', 'rejected') OR decided_by <> created_by)
        AND (state <> 'withdrawn' OR decided_by = created_by)
        AND (state <> 'rejected' OR reason <> '')
    )
);
-- No máximo uma Errata pendente por Versão: as duas mexeriam no mesmo
-- texto. É o banco que garante, mesmo com duas propostas concorrentes.
CREATE UNIQUE INDEX errata_one_pending_key ON review.errata (version_id) WHERE state = 'pending';
CREATE INDEX errata_proposition_idx ON review.errata (proposition_id, created_at);
CREATE INDEX errata_approved_idx ON review.errata (version_id, decided_at) WHERE state = 'approved';

-- queue: a Posse, uma por Proposição. Expira com 3 dias sem atividade; a
-- linha velha fica até alguém assumir de novo (expirada conta como livre).
CREATE TABLE queue.claims (
    proposition_id   uuid PRIMARY KEY REFERENCES proposition.propositions (id) ON DELETE CASCADE,
    -- Sem chave estrangeira para team.users: a conta pode ser desativada e
    -- a posse dela é ignorada (vale como livre).
    user_id          uuid NOT NULL,
    claimed_at       timestamptz NOT NULL,
    last_activity_at timestamptz NOT NULL
);

-- contribution: o que o Leitor manda sem login (CONTEXT.md,
-- "Contribuições") e os domínios cadastrados como Fonte oficial. Nada do
-- Leitor além do formulário: sem e-mail, sem nome, sem IP.
CREATE TABLE contribution.official_sources (
    id         uuid PRIMARY KEY,
    domain     text NOT NULL UNIQUE CHECK (domain <> '' AND domain = lower(domain)),
    name       text NOT NULL CHECK (name <> ''),
    sphere     text NOT NULL CHECK (sphere IN ('federal', 'state', 'municipal')),
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL
);

CREATE TABLE contribution.contributions (
    id                     uuid PRIMARY KEY,
    kind                   text NOT NULL CHECK (kind IN ('indication', 'progress_update', 'source_request')),
    state                  text NOT NULL CHECK (state IN ('pending', 'accepted', 'rejected')),
    -- sha256 do código de acompanhamento: o código mesmo só o Leitor tem.
    tracking_hash          text NOT NULL UNIQUE CHECK (length(tracking_hash) = 64),
    -- Sem chave estrangeira: módulos não enxergam as tabelas uns dos
    -- outros. Só a Atualização de tramitação tem Proposição.
    proposition_id         uuid,
    sphere                 text NOT NULL CHECK (sphere IN ('federal', 'state', 'municipal')),
    payload                jsonb NOT NULL,
    submitted_at           timestamptz NOT NULL,
    decided_by             uuid,
    decided_at             timestamptz,
    public_reason          text NOT NULL DEFAULT '',
    internal_note          text NOT NULL DEFAULT '',
    created_proposition_id uuid,
    CHECK ((kind = 'progress_update') = (proposition_id IS NOT NULL)),
    CHECK ((state = 'pending') = (decided_at IS NULL)),
    CHECK ((state = 'pending') = (decided_by IS NULL)),
    CHECK (state <> 'rejected' OR public_reason <> '')
);
CREATE INDEX contributions_pending_idx ON contribution.contributions (submitted_at) WHERE state = 'pending';

-- +goose Down
DROP SCHEMA IF EXISTS contribution CASCADE;
DROP SCHEMA IF EXISTS river CASCADE;
DROP SCHEMA IF EXISTS queue CASCADE;
DROP SCHEMA IF EXISTS review CASCADE;
DROP SCHEMA IF EXISTS progress CASCADE;
DROP SCHEMA IF EXISTS proposition CASCADE;
DROP SCHEMA IF EXISTS audit CASCADE;
DROP SCHEMA IF EXISTS team CASCADE;
