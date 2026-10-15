-- One-time password reset links issued by the GM (ADR-0008).

CREATE TABLE password_reset (
    id         TEXT    PRIMARY KEY,
    account_id TEXT    NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    token_hash TEXT    NOT NULL UNIQUE CHECK (length(token_hash) = 64),
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    used_at    INTEGER
) STRICT;

CREATE INDEX password_reset_account_id ON password_reset (account_id);
