# Модель данных

> Статус: `draft`. Хранилище: [ADR-0003](../adr/0003-sqlite-pure-go.md)

## Хранение

- Одна SQLite-БД на установку: `<data>/dndvtt.db` (WAL, foreign keys on).
- Файлы: `<data>/campaigns/<campaign_id>/assets/<sha256>.<ext>`, дедупликация по хешу.
- Идентификаторы: ULID (строка, сортируется по времени), генерирует сервер.
- Время: UTC, `INTEGER` unix-миллисекунды.
- Гибкие структуры (свойства базы, данные листа, блоки страницы): колонка `JSON` (TEXT) со схемой, версионируемой полем `schema_version`, с проверкой `CHECK (json_valid(...))`.
- Таблицы объявляются `STRICT`: SQLite проверяет типы колонок.
- Имена таблиц и колонок — `snake_case`, таблицы в единственном числе (`campaign`, `player`).
- Булевы значения — `INTEGER` `0`/`1` с `CHECK`.
- Соединение открывается с `foreign_keys=ON`, `journal_mode=WAL`, `busy_timeout=5000`, `synchronous=NORMAL`; запись в транзакциях `BEGIN IMMEDIATE`.

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
`id, name, created_at, active_scene_id?, settings JSON`
`name` — от 1 до 200 символов.

### Player
`id, campaign_id, display_name, color, role (gm|player|spectator), actor_id?, last_seen_at?, created_at`
`display_name` — от 1 до 64 символов; `color` — `#rrggbb` в нижнем регистре.
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
`id, campaign_id, token_hash, role (player|spectator), expires_at?, max_uses?, uses, revoked, created_at`
`token_hash` — sha256 токена в hex (64 символа), уникален. Инвайта с ролью `gm` нет: мастер входит с loopback (см. [security.md](security.md)).

## Миграции

- Файлы `internal/store/migrations/NNNN_description.sql` (`NNNN` — четыре цифры, описание — `[a-z0-9_]+`), встроены в бинарник, только вперёд.
- Номера идут подряд с `0001`, без пропусков и повторов; иначе приложение не стартует.
- Применённые миграции записываются в таблицу `schema_migrations (version, name, applied_at)`.
- Каждая миграция применяется в своей транзакции вместе с записью в `schema_migrations`: при ошибке не остаётся ни частичной схемы, ни записи.
- Если в БД есть версия новее, чем знает бинарник, или имя применённой миграции не совпадает с файлом, приложение не стартует (защита от запуска старой версии на новой БД и от переименования миграций).
- Любое изменение схемы — новая миграция; старые не редактируются после мерджа.
- Колонки со ссылкой на таблицу из более позднего майлстоуна (`campaign.active_scene_id`, `player.actor_id`) добавляет миграция, создающая эту таблицу: `ALTER TABLE ... ADD COLUMN ... REFERENCES ...`.
- Внутри миграции нельзя менять `PRAGMA foreign_keys`: в транзакции эта прагма игнорируется.
- JSON-поля: при изменении формата повышается `schema_version` и пишется функция апгрейда в Go.
