# Модель данных

> Статус: `draft` (раздел «Аккаунты и игровые сессии» — `review`). Хранилище: [ADR-0003](../adr/0003-sqlite-pure-go.md), доступ: [ADR-0008](../adr/0008-accounts-and-game-sessions.md)

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
Account 1─* LoginSession *─0..1 GameSession
   ├─* PasswordReset
   └─* Player (участие в кампании)

Campaign 1─* Scene 1─* Token *─0..1 Actor
   │            └─ FogMask (1:1)
   ├─* Actor (character | npc | monster) ─ sheet JSON
   ├─* Player (участник кампании) ─0..1 Actor (свой персонаж)
   ├─* GameSession (игровая встреча)
   ├─* Asset
   ├─* ChatMessage / Roll
   ├─* CombatTracker 1─* Combatant
   └─* WikiPage (дерево) ─0..1 WikiDatabase
                               └─* WikiProperty
```

### Campaign
`id, name, created_at, active_scene_id?, settings JSON`
`name` — от 1 до 200 символов.

### Аккаунты и игровые сессии

Модель и сценарии — [ADR-0008](../adr/0008-accounts-and-game-sessions.md), правила доступа — [security.md](security.md). Все `*_hash` — `sha256` токена в hex (64 символа).

#### Account
`id, login, password_hash, is_gm, created_at, password_changed_at`
`login` — 3–32 символа `[a-z0-9_.-]`, уникален. `password_hash` — argon2id в PHC-формате. `is_gm` — `0`/`1`, аккаунт с `is_gm = 1` не больше одного (частичный уникальный индекс).

#### Player
`id, campaign_id, account_id, display_name, color, role (gm|player), actor_id?, removed, last_seen_at?, created_at`
Участник кампании: связь аккаунта с кампанией, уникальная пара `(campaign_id, account_id)`. `display_name` — от 1 до 64 символов; `color` — `#rrggbb` в нижнем регистре. `removed = 1` — мастер удалил участника: строка остаётся (на неё ссылаются сообщения чата и фишки), но войти в кампанию он не может. Роль `spectator` — позже.

#### GameSession
`id, campaign_id, link_hash, started_at, ended_at?`
Одна игровая встреча. Активна, пока `ended_at IS NULL`; активной может быть не больше одной на установку (частичный уникальный индекс). `link_hash` — хеш токена ссылки на игру, уникален; «Сменить ссылку» перезаписывает его.

#### LoginSession
`id, account_id, game_session_id?, token_hash, created_at, last_seen_at, expires_at`
Вход на одном устройстве. У мастера `game_session_id` пуст; у игрока заполнен, и сессия входа действует, только пока эта игровая сессия активна. Удаление аккаунта или игровой сессии удаляет и сессии входа (`ON DELETE CASCADE`).

#### PasswordReset
`id, account_id, token_hash, created_at, expires_at, used_at?`
Одноразовая ссылка на сброс пароля, 30 минут.

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
