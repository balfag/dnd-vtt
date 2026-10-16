-- Uploaded campaign files (data-model.md, «Asset»).

CREATE TABLE asset (
    id            TEXT    PRIMARY KEY,
    campaign_id   TEXT    NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    sha256        TEXT    NOT NULL CHECK (length(sha256) = 64),
    mime          TEXT    NOT NULL CHECK (mime IN ('image/png', 'image/jpeg', 'image/webp', 'image/gif')),
    size          INTEGER NOT NULL CHECK (size > 0),
    width         INTEGER NOT NULL CHECK (width BETWEEN 1 AND 16384),
    height        INTEGER NOT NULL CHECK (height BETWEEN 1 AND 16384),
    original_name TEXT    NOT NULL CHECK (length(original_name) <= 200),
    kind          TEXT    NOT NULL CHECK (kind IN ('map', 'token', 'portrait', 'handout', 'other')),
    created_at    INTEGER NOT NULL,
    UNIQUE (campaign_id, sha256)
) STRICT;
