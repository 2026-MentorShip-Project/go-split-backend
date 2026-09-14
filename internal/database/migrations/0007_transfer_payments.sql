CREATE TABLE transfer_payments (
    event_id       BIGINT      NOT NULL REFERENCES events(id)        ON DELETE CASCADE,
    from_member_id BIGINT      NOT NULL REFERENCES event_members(id) ON DELETE CASCADE,
    to_member_id   BIGINT      NOT NULL REFERENCES event_members(id) ON DELETE CASCADE,
    paid_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (event_id, from_member_id, to_member_id),
    CHECK (from_member_id <> to_member_id)
);
