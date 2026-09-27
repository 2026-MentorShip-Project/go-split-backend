-- Scheduling returns as calendar dates. 0010 dropped the original TIMESTAMPTZ
-- columns; an event runs on a day, not at an instant, so no time or zone.
ALTER TABLE events ADD COLUMN starts_at DATE, ADD COLUMN ends_at DATE;
