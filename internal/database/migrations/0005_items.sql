CREATE TABLE items (
    id                BIGSERIAL PRIMARY KEY,
    event_id          BIGINT      NOT NULL REFERENCES events(id)        ON DELETE CASCADE,
    payer_member_id   BIGINT      NOT NULL REFERENCES event_members(id) ON DELETE RESTRICT,
    author_member_id  BIGINT      NOT NULL REFERENCES event_members(id) ON DELETE RESTRICT,
    has_receipt       BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX ON items (event_id);
CREATE INDEX ON items (payer_member_id);
CREATE INDEX ON items (author_member_id);

CREATE TABLE item_details (
    id             BIGSERIAL PRIMARY KEY,
    item_id        BIGINT      NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    ordinal        INTEGER     NOT NULL,
    name           TEXT        NOT NULL,
    amount_cents   BIGINT      NOT NULL CHECK (amount_cents >= 0),
    tag            TEXT,
    note           TEXT        NOT NULL DEFAULT '',
    -- Per-person F3 overrides. Object keyed by event_members.id (string) →
    -- fixed amount in cents. Absent members fall back to the rule-based split.
    custom_shares  JSONB       NOT NULL DEFAULT '{}'::jsonb,
    UNIQUE (item_id, ordinal)
);

CREATE INDEX ON item_details (item_id);
