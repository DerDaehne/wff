-- Long-lived tokens that let a device upload a .fit without a browser session
-- (#617). iOS has no Web Share Target, so an iOS Shortcut posts the file
-- directly — and a Shortcut cannot perform a WebAuthn ceremony.
--
-- Deliberately a separate table from sessions rather than a long-lived session:
-- this token sits in cleartext inside a Shortcut that syncs through iCloud, so
-- it must not be able to do everything a logged-in browser can. The table IS
-- the scope — it is only ever accepted by the upload endpoint.
CREATE TABLE device_tokens (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- Only the hash, like sessions and invites: the cleartext is shown once at
    -- creation and is not recoverable afterwards.
    token_hash BYTEA NOT NULL UNIQUE,
    -- Which device this belongs to, so revoking the right one is possible
    -- ("iPhone" vs "altes iPhone").
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- No expires_at on purpose: a token that dies silently mid-season breaks
    -- the Shortcut with an error nobody can read. Revocation is explicit, and
    -- last_used_at is shown so an unused token is visible and can be removed.
    last_used_at TIMESTAMPTZ
);

CREATE INDEX device_tokens_user_id_idx ON device_tokens (user_id);
