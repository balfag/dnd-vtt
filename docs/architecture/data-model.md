# Модель данных

> Статус: `draft` (разделы «Аккаунты и игровые сессии», Scene, Token, Drawing, ChatMessage, Combat, Asset, «Вики (M3)» — `review`). Хранилище: [ADR-0003](../adr/0003-sqlite-pure-go.md), доступ: [ADR-0008](../adr/0008-accounts-and-game-sessions.md)

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
   │            ├─* Drawing
   │            ├─ Combat 1─* Combatant
   │            └─ fog (JSON в Scene)
   ├─* Actor (character | npc | monster) ─ sheet JSON
   ├─* Player (участник кампании) ─0..1 Actor (свой персонаж)
   ├─* GameSession (игровая встреча)
   ├─* Asset
   ├─* ChatMessage (текст и броски)
   ├─* WikiPage (дерево; kind page | database)
   │      ├─* WikiProperty, WikiView (у базы)
   │      ├─* WikiPage-записи (дочерние базы)
   │      └─* WikiTag (через WikiPageTag), WikiLink
   └─* WikiTemplate
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

**Стол (M1).** Форматы полей в API — [protocol.md](protocol.md) («Сообщения M1: стол»); здесь — хранение и ограничения. Колонки JSON — `TEXT` с `CHECK (json_valid(...))`, ключи внутри — в `camelCase`, как в протоколе.

### Scene
`id, campaign_id, name, sort, width, height, grid JSON, background_asset_id?, fog JSON, created_at, updated_at`

- `name` — 1–200 символов; `sort` — `INTEGER`, порядок в списке (новая сцена — в конец); `width`, `height` — 100–20000 px, по умолчанию `2800`×`2100` (40×30 клеток по 70 px).
- `grid` — `{type, sizePx, unitFt, offsetX, offsetY, color, visible}`: `type` — `square` \| `hexPointy` \| `hexFlat`; `sizePx` 10–500 (по умолчанию 70); `unitFt` 1–100 (по умолчанию 5); `offsetX`, `offsetY` — `0..sizePx`; `color` — `#rrggbb`; `visible` — bool.
- `background_asset_id` → `asset(id)` `ON DELETE RESTRICT`. Если при `scene.update` задан фон, а `width`/`height` не переданы, они берутся из размеров ассета.
- `fog` — `{enabled, ops: [{op: reveal|hide, shape}]}`, не больше 500 операций; фигуры — как в протоколе (`rect`, `polygon` 3–200 точек, `circle`). Новая сцена: `{"enabled": false, "ops": []}`. Отдельной таблицы тумана нет: он всегда читается и пишется вместе со сценой.
- `campaign.active_scene_id` → `scene(id)` `ON DELETE SET NULL`; добавляется миграцией сцен (`ALTER TABLE`). Удаление активной сцены запрещено командой, `SET NULL` — страховка.

### Token
`id, scene_id, layer, x, y, w, h, rotation, z, name, name_visibility, image_asset_id?, hidden, locked, owners JSON, bars JSON, conditions JSON, created_at, updated_at`

- `scene_id` → `scene(id)` `ON DELETE CASCADE`; `image_asset_id` → `asset(id)` `ON DELETE RESTRICT`. `actor_id` добавит миграция актёров (M4).
- `layer` — `map` \| `tokens` \| `gm` (слой `drawings` — таблица `drawing`).
- `x`, `y` — `REAL`, центр в пикселях сцены, `0..width`/`0..height`; `w`, `h` — `REAL` 0.25–20 клеток; `rotation` — `REAL` `[0, 360)`, сервер нормализует; `z` — `INTEGER`, при создании без `z` — максимум на сцене + 1.
- `name` — 0–100 символов; `name_visibility` — `all` \| `hover` \| `gm`.
- `hidden`, `locked` — `0`/`1`.
- `owners` — массив `playerId` участников этой кампании (не удалённых), до 20; мастеру быть владельцем не нужно.
- `bars` — до 5 `{key, value, max, visibility}`: `key` — `[a-z0-9_]{1,16}`, уникален в фишке; `value`, `max` — целые `-9999..99999`, `max ≥ 1`; `visibility` — `all` \| `owners` \| `gm`.
- `conditions` — до 20 уникальных строк `[a-z0-9_-]{1,32}` (ключи состояний SRD; словарь — в задаче фишек на клиенте).
- Индекс `(scene_id, z)`.

### Drawing
`id, scene_id, author_player_id, kind, data JSON, color, created_at, updated_at`

- `kind` — `freehand` \| `rect` \| `circle` \| `text` \| `template`; `data` — до 16 КиБ (форматы — в протоколе); `color` — `#rrggbb`.
- `scene_id` → `scene(id)` `ON DELETE CASCADE`; `author_player_id` → `player(id)`.

### Actor
`id, campaign_id, type (character|npc|monster), name, portrait_asset_id?, owner_player_id?, sheet JSON, schema_version`
Структура `sheet` описывается в [character-sheet.md](../systems/character-sheet.md).

### ChatMessage
`id, campaign_id, game_session_id?, author_player_id?, kind, visibility, recipients JSON, payload JSON, created_at`

- `kind` — `text` \| `roll` \| `system` \| `handout` (M3); `visibility` — `public` \| `private` \| `gm` \| `self` \| `blind` (`private` — только для `text` и `handout`; `gm`/`self`/`blind` — только для `roll`; у `handout` — только `public` или `private`).
- `author_player_id` → `player(id)`; пуст у `system`. `game_session_id` → `game_session(id)` `ON DELETE SET NULL` — в какой игре написано.
- `recipients` — массив `playerId` (1–20) для `private`, иначе `[]`.
- `payload`: для `text`/`system` — `{text}` (1–2000 символов); для `roll` — `{formula, label, result}`, где `result` — `dice.Result` целиком: бросок можно показать и проверить потом; для `handout` — `{title, content}` (`title` 0–200 символов, `content` — фрагмент по схеме вики v1, до 48 КиБ).
- `chat_message_asset: message_id, asset_id` (M3) — изображения handout'а: первичный ключ `(message_id, asset_id)`, `message_id` → `chat_message(id)` `ON DELETE CASCADE`, `asset_id` → `asset(id)` `ON DELETE RESTRICT`; по ней сервер проверяет доступ игрока к ассету.
- Индекс `(campaign_id, id)`: история читается страницами по убыванию `id` (ULID сортируется по времени).
- Видимость при чтении — правила протокола; фильтр применяется к каждой строке на сервере.

### Combat / Combatant
`combat: scene_id, round, turn_combatant_id?, created_at`
`combatant: id, scene_id, token_id, initiative?, added_seq, hidden`

- Трекер — одна строка на сцену: `combat.scene_id` — первичный ключ, → `scene(id)` `ON DELETE CASCADE`; `round` ≥ 1.
- `combatant.scene_id` → `combat(scene_id)` `ON DELETE CASCADE`; `token_id` → `token(id)` `ON DELETE CASCADE` (удалённая фишка выходит из боя); уникальная пара `(scene_id, token_id)`.
- `initiative` — `INTEGER` `-99..99` или `NULL`; `added_seq` — порядок добавления (для равных значений); `hidden` — `0`/`1`.
- `turn_combatant_id` → `combatant(id)` `ON DELETE SET NULL`; при удалении участника, чей был ход, сервис сам переводит ход раньше удаления.
- `actor_id` у участника добавит M4 вместе с актёрами.

### Вики (M3)

Поведение — [gm-wiki.md](../systems/gm-wiki.md), запросы — [protocol.md](protocol.md#вики-m3). Все таблицы ниже — с `campaign_id` или ссылкой на страницу; удаление кампании удаляет вики (`ON DELETE CASCADE`).

#### WikiPage
`id, campaign_id, parent_id?, kind, title, icon, cover_asset_id?, position, content JSON, content_text, props JSON, version, created_at, updated_at, deleted_at?, trash_root_id?`

- `parent_id` → `wiki_page(id)` `ON DELETE CASCADE`; пусто — страница в корне. Глубина — до 20 уровней (проверяет сервис). **Запись базы** — страница, чей родитель имеет `kind = 'database'`; отдельной колонки для этого нет.
- `kind` — `page` \| `database`. У базы `content` — всегда пустой документ, дочерние — только записи.
- `title` — 0–200 символов; `icon` — пусто или до 8 символов (кодовых точек).
- `cover_asset_id` → `asset(id)` `ON DELETE RESTRICT`.
- `position` — `INTEGER` ≥ 0, порядок среди соседей; перемещение перенумеровывает соседей в одной транзакции (`0, 1, 2…`).
- `content` — ProseMirror JSON по схеме вики v1 (см. [protocol.md](protocol.md#содержимое-страницы)), до 1 МиБ; `content_text` — плоский текст для поиска, его строит сервер при сохранении.
- `props` — объект `{ "<propertyId>": значение }` только у записей базы, иначе `{}`. Формат значения — по типу свойства ([gm-wiki.md](../systems/gm-wiki.md#свойства)).
- `version` — `INTEGER` ≥ 1, растёт на 1 при каждом сохранении содержимого (`PUT`); теги, свойства записи и перемещение на неё не влияют.
- Корзина: `deleted_at` — время удаления, одинаковое у всего удалённого поддерева; `trash_root_id` — id страницы, с которой началось удаление (у неё самой — свой id). Окончательное удаление — `DELETE` корня корзины (поддерево уходит каскадом) через 30 дней после `deleted_at`.
- Индексы: `(campaign_id, parent_id, position)`, `(campaign_id, deleted_at)`.

#### WikiProperty / WikiView
`wiki_property: id, database_page_id, name, type, options JSON, position`
`wiki_view: id, database_page_id, name, kind, config JSON, position`

- `database_page_id` → `wiki_page(id)` `ON DELETE CASCADE` (страница с `kind = 'database'`).
- `wiki_property.name` — 1–100 символов, уникально в базе; `type` — `text` \| `number` \| `select` \| `multi_select` \| `checkbox` \| `date` \| `url` \| `relation` \| `game_link`; до 30 свойств на базу.
- `options` по типу: `number` — `{"format":"integer"|"decimal"}`; `select`/`multi_select` — `{"options":[{"id","name","color"}]}` (`color` — `gray` \| `brown` \| `orange` \| `yellow` \| `green` \| `blue` \| `purple` \| `red`); `relation` — `{"targetDatabaseId"}`; остальные — `{}`.
- `wiki_view.name` — 1–100 символов; `kind` — `table` \| `cards`; 1–10 видов на базу.
- `config`: `{"filters":[{"propertyId"|"field","op","value"?}], "sorts":[{"propertyId"|"field","dir":"asc"|"desc"}], "visible":["<propertyId>"], "widths":{"<propertyId>|title": px}}`; `field` — `title` \| `createdAt` \| `updatedAt` \| `tags`. Сервер проверяет форму и ссылки на свойства этой базы; при удалении свойства вычищает ссылки на него из видов.

#### WikiTag / WikiPageTag
`wiki_tag: id, campaign_id, name, name_key`
`wiki_page_tag: page_id, tag_id`

- `name` — 1–40 символов, как ввёл мастер; `name_key` — `strings.ToLower(name)` в Go (`COLLATE NOCASE` в SQLite не понимает кириллицу); уникальная пара `(campaign_id, name_key)`.
- `wiki_page_tag` — первичный ключ `(page_id, tag_id)`, обе ссылки `ON DELETE CASCADE`; до 20 тегов на страницу.

#### WikiLink и ссылки на ассеты
`wiki_link: page_id, target_kind, target_id, source`
`wiki_page_asset: page_id, asset_id`
`wiki_template_asset: template_id, asset_id`

- Сервер пересчитывает их при каждом сохранении содержимого, свойств записи и шаблона: старые строки владельца удаляются, новые вставляются в той же транзакции.
- `wiki_link` — для обратных ссылок и текущих имён упоминаний: `target_kind` — `page` \| `scene` \| `token`; `source` — `mention` (упоминание в содержимом) \| `property` (значение `relation` или `game_link` записи); первичный ключ `(page_id, target_kind, target_id, source)`; индекс `(target_kind, target_id)`. На `target_id` внешнего ключа нет: удалённый объект — ссылка без цели.
- `wiki_page_asset` / `wiki_template_asset` — изображения в содержимом: `asset_id` → `asset(id)` `ON DELETE RESTRICT` (ассет, который использует вики, в том числе страница в корзине или шаблон, удалить нельзя — `409`); владелец — `ON DELETE CASCADE`.

#### WikiTemplate
`id, campaign_id, name, icon, content JSON, created_at`

- Свой шаблон страницы мастера. `name` — 1–100 символов; `icon` и `content` — как у страницы. До 100 шаблонов на кампанию. Встроенные шаблоны в БД не хранятся.

#### Полнотекстовый поиск
`wiki_fts` — виртуальная таблица FTS5 `(page_id UNINDEXED, title, body)` с `tokenize='unicode61 remove_diacritics 2'`. Сервис обновляет строку страницы в той же транзакции, что и саму страницу; страница в корзине из `wiki_fts` удаляется, при восстановлении добавляется снова.

### Asset
`id, campaign_id, sha256, mime, size, width, height, original_name, kind, created_at`

- `sha256` — hex 64 символа; уникальная пара `(campaign_id, sha256)`: повторная загрузка того же файла в кампанию возвращает существующую строку.
- `mime` — `image/png` \| `image/jpeg` \| `image/webp` \| `image/gif` (аудио — позже); `size` — байты, до 50 МиБ; `width`, `height` — px, 1–16384.
- `original_name` — до 200 символов, только для показа, на диск не влияет.
- `kind` — `map` \| `token` \| `portrait` \| `handout` \| `other`.
- `campaign_id` → `campaign(id)` `ON DELETE CASCADE`. Ссылки на ассет (`scene.background_asset_id`, `token.image_asset_id`) — `ON DELETE RESTRICT`: используемый ассет удалить нельзя (`409`).
- Файл: `<data>/campaigns/<campaign_id>/assets/<sha256>.<ext>`.

## Миграции

- Файлы `internal/store/migrations/NNNN_description.sql` (`NNNN` — четыре цифры, описание — `[a-z0-9_]+`), встроены в бинарник, только вперёд.
- Номера идут подряд с `0001`, без пропусков и повторов; иначе приложение не стартует.
- Применённые миграции записываются в таблицу `schema_migrations (version, name, applied_at)`.
- Каждая миграция применяется в своей транзакции вместе с записью в `schema_migrations`: при ошибке не остаётся ни частичной схемы, ни записи.
- Если в БД есть версия новее, чем знает бинарник, или имя применённой миграции не совпадает с файлом, приложение не стартует (защита от запуска старой версии на новой БД и от переименования миграций).
- Любое изменение схемы — новая миграция; старые не редактируются после мерджа.
- Колонки со ссылкой на таблицу из более позднего майлстоуна (`campaign.active_scene_id`, `player.actor_id`) добавляет миграция, создающая эту таблицу: `ALTER TABLE ... ADD COLUMN ... REFERENCES ...`.
- Внутри миграции нельзя менять `PRAGMA foreign_keys`: в транзакции эта прагма игнорируется.
- Миграции M1: `0005_assets` (`asset`), `0006_scenes` (`scene`, `token`, `drawing`, `campaign.active_scene_id`), `0007_chat` (`chat_message`), `0008_combat` (`combat`, `combatant`). Номера закреплены за задачами: если задачи мерджатся не по порядку, исполнитель следующей задачи берёт следующий свободный номер и пишет об этом в «Отклонениях».
- Миграции M3: `0009_wiki` (все таблицы `wiki_*` и `wiki_fts`), `0010_handouts` (пересборка `chat_message` с новыми `CHECK` для `handout` и таблица `chat_message_asset`). Пересборка — по схеме SQLite «создать новую таблицу, скопировать строки, удалить старую, переименовать» в одной транзакции; на `chat_message` никто не ссылается внешним ключом, поэтому `PRAGMA foreign_keys` трогать не нужно (и нельзя, см. выше); индекс `chat_message_campaign_id` создаётся заново.
- JSON-поля: при изменении формата повышается `schema_version` и пишется функция апгрейда в Go.
