-- Handouts in the chat (data-model.md, «ChatMessage»): new CHECKs need a table rebuild.

CREATE TABLE chat_message_new (
    id               TEXT    PRIMARY KEY,
    campaign_id      TEXT    NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    game_session_id  TEXT    REFERENCES game_session (id) ON DELETE SET NULL,
    author_player_id TEXT    REFERENCES player (id),
    kind             TEXT    NOT NULL CHECK (kind IN ('text', 'roll', 'system', 'handout')),
    visibility       TEXT    NOT NULL CHECK (visibility IN ('public', 'private', 'gm', 'self', 'blind')),
    recipients       TEXT    NOT NULL CHECK (json_valid(recipients)),
    payload          TEXT    NOT NULL CHECK (json_valid(payload)),
    created_at       INTEGER NOT NULL,
    CHECK (visibility <> 'private' OR kind IN ('text', 'handout')),
    CHECK (visibility NOT IN ('gm', 'self', 'blind') OR kind = 'roll'),
    CHECK (kind <> 'handout' OR visibility IN ('public', 'private'))
) STRICT;
INSERT INTO chat_message_new (id, campaign_id, game_session_id, author_player_id, kind, visibility, recipients, payload, created_at)
    SELECT id, campaign_id, game_session_id, author_player_id, kind, visibility, recipients, payload, created_at FROM chat_message;
DROP TABLE chat_message;
ALTER TABLE chat_message_new RENAME TO chat_message;
CREATE INDEX chat_message_campaign_id ON chat_message (campaign_id, id);

CREATE TABLE chat_message_asset (
    message_id TEXT NOT NULL REFERENCES chat_message (id) ON DELETE CASCADE,
    asset_id   TEXT NOT NULL REFERENCES asset (id) ON DELETE RESTRICT,
    PRIMARY KEY (message_id, asset_id)
) STRICT;
CREATE INDEX chat_message_asset_asset ON chat_message_asset (asset_id);
