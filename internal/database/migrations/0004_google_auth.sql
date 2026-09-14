-- Hosts may sign in with Google instead of a password. Such accounts have no
-- password_hash; google_sub is Google's stable account id from the ID token.
ALTER TABLE hosts ALTER COLUMN password_hash DROP NOT NULL;
ALTER TABLE hosts ADD COLUMN google_sub TEXT UNIQUE;
