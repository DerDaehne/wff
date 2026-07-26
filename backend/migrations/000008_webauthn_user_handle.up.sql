-- The WebAuthn user handle used at registration must be reused verbatim at
-- every subsequent login: discoverable/resident credentials return it as
-- userHandle in the assertion, and go-webauthn rejects a login where it
-- doesn't match user.WebAuthnID(). DEFAULT keeps ad-hoc test/seed inserts
-- (that don't set it explicitly) working — real registration always passes
-- its own value explicitly, overriding the default.
ALTER TABLE users ADD COLUMN webauthn_user_handle BYTEA NOT NULL DEFAULT uuid_send(gen_random_uuid());
ALTER TABLE users ADD CONSTRAINT users_webauthn_user_handle_unique UNIQUE (webauthn_user_handle);
