-- Initiative tracker of a scene (data-model.md, «Combat / Combatant»).

CREATE TABLE combat (
    scene_id          TEXT    PRIMARY KEY REFERENCES scene (id) ON DELETE CASCADE,
    round             INTEGER NOT NULL CHECK (round >= 1),
    turn_combatant_id TEXT    REFERENCES combatant (id) ON DELETE SET NULL,
    created_at        INTEGER NOT NULL
) STRICT;

CREATE TABLE combatant (
    id         TEXT    PRIMARY KEY,
    scene_id   TEXT    NOT NULL REFERENCES combat (scene_id) ON DELETE CASCADE,
    token_id   TEXT    NOT NULL REFERENCES token (id) ON DELETE CASCADE,
    initiative INTEGER CHECK (initiative BETWEEN -99 AND 99),
    added_seq  INTEGER NOT NULL,
    hidden     INTEGER NOT NULL CHECK (hidden IN (0, 1)),
    UNIQUE (scene_id, token_id)
) STRICT;
CREATE INDEX combatant_token_id ON combatant (token_id);
