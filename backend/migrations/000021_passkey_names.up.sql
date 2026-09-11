-- Passkeys had no way to tell them apart once you have more than one
-- (phone, laptop, security key, ...) — the management UI needs a label per
-- credential. Registration always sets a real value explicitly; the default
-- only covers the three credentials that already existed before this
-- migration.
ALTER TABLE webauthn_credentials ADD COLUMN name TEXT NOT NULL DEFAULT 'Passkey';
