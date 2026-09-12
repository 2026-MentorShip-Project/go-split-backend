CREATE TABLE event_item_tags (
    id       BIGSERIAL PRIMARY KEY,
    event_id BIGINT      NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    label    TEXT        NOT NULL,
    ordinal  INTEGER     NOT NULL,
    UNIQUE (event_id, label)
);
CREATE INDEX ON event_item_tags (event_id);

CREATE TABLE event_cond_tags (
    id       BIGSERIAL PRIMARY KEY,
    event_id BIGINT      NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    label    TEXT        NOT NULL,
    ordinal  INTEGER     NOT NULL,
    UNIQUE (event_id, label)
);
CREATE INDEX ON event_cond_tags (event_id);

CREATE TABLE event_rules (
    id       BIGSERIAL PRIMARY KEY,
    event_id BIGINT      NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    item_tag TEXT        NOT NULL,
    -- Ordered list of {conds:[label,...], mode:"exclude"|"weight", weight:number?}.
    -- First group whose conds are all present on a member wins for that member.
    groups   JSONB       NOT NULL DEFAULT '[]'::jsonb,
    -- Fallback for members that matched no group. NULL means weight 1
    -- (equal share). Same {mode, weight?} shape as a group entry.
    rest     JSONB,
    ordinal  INTEGER     NOT NULL,
    UNIQUE (event_id, item_tag)
);
CREATE INDEX ON event_rules (event_id);
