-- Campaigns. Participants, accounts and invites get their own migrations.
-- Columns that reference tables from later milestones (campaign.active_scene_id)
-- are added by the migrations that create those tables.

CREATE TABLE campaign (
    id         TEXT    PRIMARY KEY,
    name       TEXT    NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    settings   TEXT    NOT NULL DEFAULT '{}' CHECK (json_valid(settings)),
    created_at INTEGER NOT NULL
) STRICT;
