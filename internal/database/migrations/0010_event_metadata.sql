-- Event scheduling is no longer part of the product.
-- Existing immutable settlement JSON retains its original audit metadata.
ALTER TABLE events DROP COLUMN starts_at, DROP COLUMN ends_at;
ALTER TABLE events ADD COLUMN transfer_note TEXT NOT NULL DEFAULT '';
