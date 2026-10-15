# Модель данных

> Статус: `draft`. Хранилище: [ADR-0003](../adr/0003-sqlite-pure-go.md)

## Хранение

- Одна SQLite-БД на установку: `<data>/dndvtt.db` (WAL, foreign keys on).
- Файлы: `<data>/campaigns/<campaign_id>/assets/<sha256>.<ext>`, дедупликация по хешу.
- Идентификаторы: ULID (строка, сортируется по времени), генерирует сервер.
- Время: UTC, `INTEGER` unix-миллисекунды.
- Гибкие структуры (свойства базы, данные листа, блоки страницы): колонка `JSON` (TEXT) со схемой, версионируемой полем `schema_version`.

## Сущности

```
Campaign 1─* Scene 1─* Token *─0..1 Actor
   │            └─ FogMask (1:1)
   ├─* Actor (character | npc | monster) ─ sheet JSON
   ├─* Player (участник кампании) ─0..1 Actor (свой персонаж)
   ├─* Invite
   ├─* Asset
   ├─* ChatMessage / Roll
   ├─* CombatTracker 1─* Combatant
   └─* WikiPage (дерево) ─0..1 WikiDatabase
                               └─* WikiProperty
```

### Campaign
`id, name, created_at, active_scene_id, settings JSON`

### Player
`id, campaign_id, display_name, color, role (gm|player|spectator), actor_id?, last_seen_at`
Игрок привязан к кампании и идентифицируется токеном сессии (см. [security.md](security.md)).

### Scene
`id, campaign_id, name, sort, background_asset_id?, width, height, grid JSON {type: square|hex, size_px, unit_ft, offset_x, offset_y, color, visible}, settings JSON`

### Token
`id, scene_id, actor_id?, layer (map|tokens|gm|drawings), x, y, w, h, rotation, image_asset_id?, name, hidden, locked, owners JSON [player_id], bars JSON, z, updated_at`
`owners` — кто из игроков может двигать фишку (мастер может всегда).

### FogMask
`scene_id, enabled, revealed JSON` — список раскрытых полигонов и прямоугольников (векторно, а не растром, чтобы не зависеть от разрешения).

### Actor
`id, campaign_id, type (character|npc|monster), name, portrait_asset_id?, owner_player_id?, sheet JSON, schema_version`
Структура `sheet` описывается в [character-sheet.md](../systems/character-sheet.md).

### ChatMessage
`id, campaign_id, author_player_id, kind (text|roll|system|handout), visibility (public|gm|self|blind), payload JSON, created_at`
Бросок хранит формулу, отдельные результаты кубиков и итог, чтобы его можно было воспроизвести и проверить.

### CombatTracker / Combatant
`tracker: id, scene_id, round, turn_index, active`
`combatant: id, tracker_id, token_id?, actor_id?, initiative, sort, hidden`

### WikiPage
`id, campaign_id, parent_id?, title, icon, sort, content JSON (ProseMirror doc), database_id? (если страница — запись базы), props JSON (значения свойств записи), created_at, updated_at`

### WikiDatabase / WikiProperty / WikiView
`database: id, page_id (страница-контейнер), name`
`property: id, database_id, name, type (text|number|select|multi_select|checkbox|date|relation|url), options JSON, sort`
`view: id, database_id, name, kind (table|cards), filters JSON, sorts JSON, visible_props JSON`

### Asset
`id, campaign_id, sha256, mime, size, original_name, kind (map|token|portrait|handout|audio|other), created_at`

### Invite
`id, campaign_id, token_hash, role, expires_at?, max_uses?, uses, revoked`

## Миграции

- Файлы `internal/store/migrations/NNNN_description.sql`, только вперёд.
- Любое изменение схемы — новая миграция; старые не редактируются после мерджа.
- JSON-поля: при изменении формата повышается `schema_version` и пишется функция апгрейда в Go.
