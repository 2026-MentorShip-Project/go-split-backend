CREATE EXTENSION IF NOT EXISTS citext;
-- Auth-adjacent tables.
--
-- Hosts have real accounts; guests (co-organizers and participants) are
-- session-bound with email + phone as recovery markers only. Events, event
-- members, and invitations live here too because the invite-code flows in
-- POST /auth/join and POST /auth/recover need to look them up.

CREATE TABLE hosts (
    id            BIGSERIAL PRIMARY KEY,
    name          TEXT        NOT NULL,
    email         CITEXT      NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE guests (
    id         BIGSERIAL PRIMARY KEY,
    email      CITEXT      NOT NULL,
    phone      TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (email, phone)
);

CREATE TABLE events (
    id         BIGSERIAL PRIMARY KEY,
    host_id    BIGINT      NOT NULL REFERENCES hosts(id) ON DELETE RESTRICT,
    name       TEXT        NOT NULL,
    place      TEXT        NOT NULL DEFAULT '',
    starts_at  TIMESTAMPTZ,
    ends_at    TIMESTAMPTZ,
    template   TEXT        NOT NULL DEFAULT '自訂',
    settled    BOOLEAN     NOT NULL DEFAULT FALSE,
    archived   BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TYPE event_role AS ENUM ('host', 'co', 'member');

CREATE TABLE event_members (
    id         BIGSERIAL PRIMARY KEY,
    event_id   BIGINT      NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    host_id    BIGINT      REFERENCES hosts(id)  ON DELETE CASCADE,
    guest_id   BIGINT      REFERENCES guests(id) ON DELETE CASCADE,
    display    TEXT        NOT NULL,
    role       event_role  NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Exactly one of host_id / guest_id must be set.
    CHECK ((host_id IS NULL) <> (guest_id IS NULL)),
    UNIQUE (event_id, host_id),
    UNIQUE (event_id, guest_id)
);

CREATE TABLE invitations (
    code       TEXT        PRIMARY KEY,
    event_id   BIGINT      NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Sessions cover both hosts and guests: exactly one of host_id / guest_id is
-- set. The token is the opaque cookie value; expires_at is enforced at read
-- time in the app layer.
CREATE TABLE sessions (
    token      TEXT        PRIMARY KEY,
    host_id    BIGINT      REFERENCES hosts(id)  ON DELETE CASCADE,
    guest_id   BIGINT      REFERENCES guests(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    CHECK ((host_id IS NULL) <> (guest_id IS NULL))
);

CREATE INDEX ON sessions (host_id);
CREATE INDEX ON sessions (guest_id);
CREATE INDEX ON sessions (expires_at);
CREATE INDEX ON event_members (event_id);
CREATE INDEX ON invitations (event_id);
