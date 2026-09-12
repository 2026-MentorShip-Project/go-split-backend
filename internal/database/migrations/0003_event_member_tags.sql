ALTER TABLE event_members ADD COLUMN tags TEXT[] NOT NULL DEFAULT '{}';

-- Group page needs to add "seat-holder" members before a guest joins by
-- invite code, so relax the identity CHECK from exactly-one to at-most-one.
DO $$
DECLARE
    cname TEXT;
BEGIN
    SELECT conname INTO cname
      FROM pg_constraint
     WHERE conrelid = 'event_members'::regclass
       AND contype  = 'c'
     LIMIT 1;
    IF cname IS NOT NULL THEN
        EXECUTE format('ALTER TABLE event_members DROP CONSTRAINT %I', cname);
    END IF;
END $$;

ALTER TABLE event_members
    ADD CONSTRAINT event_members_at_most_one_identity
    CHECK (host_id IS NULL OR guest_id IS NULL);
