-- Members join for themselves; hosts no longer reserve a seat and merge a
-- joiner into it, so the flag that gated binding has no remaining reader.
ALTER TABLE event_members DROP COLUMN virtual;
