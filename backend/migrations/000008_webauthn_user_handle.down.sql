ALTER TABLE users DROP CONSTRAINT users_webauthn_user_handle_unique;
ALTER TABLE users DROP COLUMN webauthn_user_handle;
