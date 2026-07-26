-- go-webauthn requires BackupEligible to be identical between registration
-- and every subsequent login (protocol invariant, "this should NEVER
-- change" per the library's own doc comment) — reconstructing a Credential
-- without it defaults to false, which mismatches any real backup-eligible
-- passkey (the vast majority of modern platform/password-manager passkeys)
-- and fails every login with "Backup Eligible flag inconsistency detected".
ALTER TABLE webauthn_credentials ADD COLUMN backup_eligible BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE webauthn_credentials ADD COLUMN backup_state BOOLEAN NOT NULL DEFAULT false;
