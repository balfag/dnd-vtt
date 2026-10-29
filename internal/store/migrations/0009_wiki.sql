-- Wiki pages, databases, tags, links, templates and search (data-model.md, «Вики (M3)»).

CREATE TABLE wiki_page (
    id             TEXT    PRIMARY KEY,
    campaign_id    TEXT    NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    parent_id      TEXT    REFERENCES wiki_page (id) ON DELETE CASCADE,
    kind           TEXT    NOT NULL CHECK (kind IN ('page', 'database')),
    title          TEXT    NOT NULL CHECK (length(title) <= 200),
    icon           TEXT    NOT NULL DEFAULT '',
    cover_asset_id TEXT    REFERENCES asset (id) ON DELETE RESTRICT,
    position       INTEGER NOT NULL CHECK (position >= 0),
    content        TEXT    NOT NULL CHECK (json_valid(content)),
    content_text   TEXT    NOT NULL,
    props          TEXT    NOT NULL DEFAULT '{}' CHECK (json_valid(props) AND json_type(props) = 'object'),
    version        INTEGER NOT NULL CHECK (version >= 1),
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,
    deleted_at     INTEGER,
    trash_root_id  TEXT,
    CHECK ((deleted_at IS NULL) = (trash_root_id IS NULL))
) STRICT;
CREATE INDEX wiki_page_tree ON wiki_page (campaign_id, parent_id, position);
CREATE INDEX wiki_page_trash ON wiki_page (campaign_id, deleted_at);
CREATE INDEX wiki_page_parent ON wiki_page (parent_id);
CREATE INDEX wiki_page_cover ON wiki_page (cover_asset_id);

CREATE TABLE wiki_property (
    id               TEXT    PRIMARY KEY,
    database_page_id TEXT    NOT NULL REFERENCES wiki_page (id) ON DELETE CASCADE,
    name             TEXT    NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    type             TEXT    NOT NULL CHECK (type IN ('text', 'number', 'select', 'multi_select', 'checkbox', 'date', 'url', 'relation', 'game_link')),
    options          TEXT    NOT NULL DEFAULT '{}' CHECK (json_valid(options)),
    position         INTEGER NOT NULL CHECK (position >= 0),
    UNIQUE (database_page_id, name)
) STRICT;

CREATE TABLE wiki_view (
    id               TEXT    PRIMARY KEY,
    database_page_id TEXT    NOT NULL REFERENCES wiki_page (id) ON DELETE CASCADE,
    name             TEXT    NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    kind             TEXT    NOT NULL CHECK (kind IN ('table', 'cards')),
    config           TEXT    NOT NULL CHECK (json_valid(config)),
    position         INTEGER NOT NULL CHECK (position >= 0)
) STRICT;
CREATE INDEX wiki_view_db ON wiki_view (database_page_id, position);

CREATE TABLE wiki_tag (
    id          TEXT PRIMARY KEY,
    campaign_id TEXT NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    name        TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 40),
    name_key    TEXT NOT NULL,
    UNIQUE (campaign_id, name_key)
) STRICT;

CREATE TABLE wiki_page_tag (
    page_id TEXT NOT NULL REFERENCES wiki_page (id) ON DELETE CASCADE,
    tag_id  TEXT NOT NULL REFERENCES wiki_tag (id) ON DELETE CASCADE,
    PRIMARY KEY (page_id, tag_id)
) STRICT;
CREATE INDEX wiki_page_tag_tag ON wiki_page_tag (tag_id);

CREATE TABLE wiki_link (
    page_id     TEXT NOT NULL REFERENCES wiki_page (id) ON DELETE CASCADE,
    target_kind TEXT NOT NULL CHECK (target_kind IN ('page', 'scene', 'token')),
    target_id   TEXT NOT NULL,
    source      TEXT NOT NULL CHECK (source IN ('mention', 'property')),
    PRIMARY KEY (page_id, target_kind, target_id, source)
) STRICT;
CREATE INDEX wiki_link_target ON wiki_link (target_kind, target_id);

CREATE TABLE wiki_page_asset (
    page_id  TEXT NOT NULL REFERENCES wiki_page (id) ON DELETE CASCADE,
    asset_id TEXT NOT NULL REFERENCES asset (id) ON DELETE RESTRICT,
    PRIMARY KEY (page_id, asset_id)
) STRICT;
CREATE INDEX wiki_page_asset_asset ON wiki_page_asset (asset_id);

CREATE TABLE wiki_template (
    id          TEXT    PRIMARY KEY,
    campaign_id TEXT    NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    name        TEXT    NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    icon        TEXT    NOT NULL DEFAULT '',
    content     TEXT    NOT NULL CHECK (json_valid(content)),
    created_at  INTEGER NOT NULL
) STRICT;
CREATE INDEX wiki_template_campaign ON wiki_template (campaign_id);

CREATE TABLE wiki_template_asset (
    template_id TEXT NOT NULL REFERENCES wiki_template (id) ON DELETE CASCADE,
    asset_id    TEXT NOT NULL REFERENCES asset (id) ON DELETE RESTRICT,
    PRIMARY KEY (template_id, asset_id)
) STRICT;
CREATE INDEX wiki_template_asset_asset ON wiki_template_asset (asset_id);

CREATE VIRTUAL TABLE wiki_fts USING fts5 (page_id UNINDEXED, title, body, tokenize = 'unicode61 remove_diacritics 2');
