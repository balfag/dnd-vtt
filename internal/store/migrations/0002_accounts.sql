-- Local accounts and their per-device login sessions (ADR-0008).
-- login_session.game_session_id is added by the migration that creates game_session.

CREATE TABLE account (
    id                  TEXT    PRIMARY KEY,
    login               TEXT    NOT NULL UNIQUE CHECK (length(login) BETWEEN 3 AND 32 AND login NOT GLOB '*[^a-z0-9_.-]*'),
    password_hash       TEXT    NOT NULL CHECK (password_hash GLOB '$argon2id$*'),
    is_gm               INTEGER NOT NULL DEFAULT 0 CHECK (is_gm IN (0, 1)),
    created_at          INTEGER NOT NULL,
    password_changed_at INTEGER NOT NULL
) STRICT;

-- At most one GM account per installation.
CREATE UNIQUE INDEX account_single_gm ON account (is_gm) WHERE is_gm = 1;

CREATE TABLE login_session (
    id           TEXT    PRIMARY KEY,
    account_id   TEXT    NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    token_hash   TEXT    NOT NULL UNIQUE CHECK (length(token_hash) = 64),
    created_at   INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL,
    expires_at   INTEGER NOT NULL
) STRICT;

CREATE INDEX login_session_account_id ON login_session (account_id);
