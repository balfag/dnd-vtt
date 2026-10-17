-- Chat messages and rolls (data-model.md, «ChatMessage»).

CREATE TABLE chat_message (
    id               TEXT    PRIMARY KEY,
    campaign_id      TEXT    NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    game_session_id  TEXT    REFERENCES game_session (id) ON DELETE SET NULL,
    author_player_id TEXT    REFERENCES player (id),
    kind             TEXT    NOT NULL CHECK (kind IN ('text', 'roll', 'system')),
    visibility       TEXT    NOT NULL CHECK (visibility IN ('public', 'private', 'gm', 'self', 'blind')),
    recipients       TEXT    NOT NULL CHECK (json_valid(recipients)),
    payload          TEXT    NOT NULL CHECK (json_valid(payload)),
    created_at       INTEGER NOT NULL,
    CHECK (visibility <> 'private' OR kind = 'text'),
    CHECK (visibility NOT IN ('gm', 'self', 'blind') OR kind = 'roll')
) STRICT;
CREATE INDEX chat_message_campaign_id ON chat_message (campaign_id, id);
