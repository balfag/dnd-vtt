-- Table of M1: scenes, tokens and drawings (data-model.md, «Стол (M1)»).

CREATE TABLE scene (
    id                  TEXT    PRIMARY KEY,
    campaign_id         TEXT    NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    name                TEXT    NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    sort                INTEGER NOT NULL,
    width               INTEGER NOT NULL CHECK (width BETWEEN 100 AND 20000),
    height              INTEGER NOT NULL CHECK (height BETWEEN 100 AND 20000),
    grid                TEXT    NOT NULL CHECK (json_valid(grid)),
    background_asset_id TEXT    REFERENCES asset (id) ON DELETE RESTRICT,
    fog                 TEXT    NOT NULL DEFAULT '{"enabled":false,"ops":[]}' CHECK (json_valid(fog)),
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL
) STRICT;
CREATE INDEX scene_campaign_sort ON scene (campaign_id, sort);
CREATE INDEX scene_background_asset_id ON scene (background_asset_id);

CREATE TABLE token (
    id              TEXT    PRIMARY KEY,
    scene_id        TEXT    NOT NULL REFERENCES scene (id) ON DELETE CASCADE,
    layer           TEXT    NOT NULL CHECK (layer IN ('map', 'tokens', 'gm')),
    x               REAL    NOT NULL,
    y               REAL    NOT NULL,
    w               REAL    NOT NULL CHECK (w BETWEEN 0.25 AND 20),
    h               REAL    NOT NULL CHECK (h BETWEEN 0.25 AND 20),
    rotation        REAL    NOT NULL CHECK (rotation >= 0 AND rotation < 360),
    z               INTEGER NOT NULL,
    name            TEXT    NOT NULL CHECK (length(name) <= 100),
    name_visibility TEXT    NOT NULL CHECK (name_visibility IN ('all', 'hover', 'gm')),
    image_asset_id  TEXT    REFERENCES asset (id) ON DELETE RESTRICT,
    hidden          INTEGER NOT NULL CHECK (hidden IN (0, 1)),
    locked          INTEGER NOT NULL CHECK (locked IN (0, 1)),
    owners          TEXT    NOT NULL CHECK (json_valid(owners)),
    bars            TEXT    NOT NULL CHECK (json_valid(bars)),
    conditions      TEXT    NOT NULL CHECK (json_valid(conditions)),
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
) STRICT;
CREATE INDEX token_scene_z ON token (scene_id, z);
CREATE INDEX token_image_asset_id ON token (image_asset_id);

CREATE TABLE drawing (
    id               TEXT    PRIMARY KEY,
    scene_id         TEXT    NOT NULL REFERENCES scene (id) ON DELETE CASCADE,
    author_player_id TEXT    NOT NULL REFERENCES player (id),
    kind             TEXT    NOT NULL CHECK (kind IN ('freehand', 'rect', 'circle', 'text', 'template')),
    data             TEXT    NOT NULL CHECK (json_valid(data)),
    color            TEXT    NOT NULL CHECK (color GLOB '#[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]'),
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL
) STRICT;
CREATE INDEX drawing_scene_id ON drawing (scene_id);

ALTER TABLE campaign ADD COLUMN active_scene_id TEXT REFERENCES scene (id) ON DELETE SET NULL;
