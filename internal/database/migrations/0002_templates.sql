-- Reference template catalog for the create-event flow.
--
-- One row per template; `label` is the visible name shown in the picker
-- (matches events.template so a lookup joins on that column). `content` is
-- the full body: item_tags, cond_tags, and the ordered rules the split
-- engine reads at compute time. Kept as JSONB so the shape can evolve
-- without a migration per rule change.
CREATE TABLE templates (
    label       TEXT        PRIMARY KEY,
    description TEXT        NOT NULL DEFAULT '',
    content     JSONB       NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
