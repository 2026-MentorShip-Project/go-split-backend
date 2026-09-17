-- Preserve existing identities and relationships while separating account
-- identity from the event-specific host role. Historical migrations stay intact.
ALTER TABLE hosts RENAME TO accounts;
ALTER SEQUENCE hosts_id_seq RENAME TO accounts_id_seq;
ALTER TABLE accounts RENAME CONSTRAINT hosts_pkey TO accounts_pkey;
ALTER TABLE accounts RENAME CONSTRAINT hosts_email_key TO accounts_email_key;
ALTER TABLE accounts RENAME CONSTRAINT hosts_google_sub_key TO accounts_google_sub_key;

ALTER TABLE events RENAME COLUMN host_id TO account_id;
ALTER TABLE events RENAME CONSTRAINT events_host_id_fkey TO events_account_id_fkey;

ALTER TABLE event_members RENAME COLUMN host_id TO account_id;
ALTER TABLE event_members RENAME CONSTRAINT event_members_host_id_fkey TO event_members_account_id_fkey;
ALTER TABLE event_members RENAME CONSTRAINT event_members_event_id_host_id_key TO event_members_event_id_account_id_key;

ALTER TABLE sessions RENAME COLUMN host_id TO account_id;
ALTER TABLE sessions RENAME CONSTRAINT sessions_host_id_fkey TO sessions_account_id_fkey;
ALTER INDEX sessions_host_id_idx RENAME TO sessions_account_id_idx;
