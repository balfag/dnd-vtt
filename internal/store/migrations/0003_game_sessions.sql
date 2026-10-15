-- Campaign participants, game sessions (ADR-0008) and binding of player login
-- sessions to the game session they were created for.

CREATE TABLE player (
    id           TEXT    PRIMARY KEY,
    campaign_id  TEXT    NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    account_id   TEXT    NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    display_name TEXT    NOT NULL CHECK (length(display_name) BETWEEN 1 AND 64),
    color        TEXT    NOT NULL CHECK (color GLOB '#[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]'),
    role         TEXT    NOT NULL CHECK (role IN ('gm', 'player')),
    removed      INTEGER NOT NULL DEFAULT 0 CHECK (removed IN (0, 1)),
    last_seen_at INTEGER,
    created_at   INTEGER NOT NULL,
    UNIQUE (campaign_id, account_id)
) STRICT;

CREATE INDEX player_account_id ON player (account_id);

CREATE TABLE game_session (
    id          TEXT    PRIMARY KEY,
    campaign_id TEXT    NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    link_hash   TEXT    NOT NULL UNIQUE CHECK (length(link_hash) = 64),
    started_at  INTEGER NOT NULL,
    ended_at    INTEGER
) STRICT;

-- At most one active game session per installation.
CREATE UNIQUE INDEX game_session_single_active ON game_session ((ended_at IS NULL)) WHERE ended_at IS NULL;
CREATE INDEX game_session_campaign_id ON game_session (campaign_id);

-- NULL for GM sessions; a player session dies with its game session.
ALTER TABLE login_session ADD COLUMN game_session_id TEXT REFERENCES game_session (id) ON DELETE CASCADE;
CREATE INDEX login_session_game_session_id ON login_session (game_session_id);
