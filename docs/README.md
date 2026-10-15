# Документация

| Раздел | Что внутри |
|--------|------------|
| [vision.md](vision.md) | Зачем проект, для кого, ключевые сценарии, что мы **не** делаем |
| [glossary.md](glossary.md) | Единый словарь терминов RU↔EN |
| [roadmap.md](roadmap.md) | Майлстоуны M0–M5 и их состав |

## Архитектура
- [overview.md](architecture/overview.md): компоненты, стек, структура кода, жизненный цикл приложения
- [networking.md](architecture/networking.md): порты, проброс, туннель, STUN/TURN
- [data-model.md](architecture/data-model.md): сущности и хранение
- [protocol.md](architecture/protocol.md): WebSocket-протокол клиент↔сервер
- [security.md](architecture/security.md): аккаунты, вход, роли, видимость данных

## Системы
- [vtt.md](systems/vtt.md): виртуальный стол
- [voice-video.md](systems/voice-video.md): голос и видео
- [character-sheet.md](systems/character-sheet.md): лист персонажа (черновик)
- [gm-wiki.md](systems/gm-wiki.md): вики мастера

## Решения (ADR)
- [0000-template.md](adr/0000-template.md): шаблон
- [0001](adr/0001-go-server-ts-client.md): Go-сервер + TypeScript-клиент в одном бинарнике
- [0002](adr/0002-embedded-sfu-pion.md): встроенный SFU на pion
- [0003](adr/0003-sqlite-pure-go.md): SQLite без CGO
- [0004](adr/0004-dnd-5e-srd-5-2.md): D&D 5e 2024 / SRD 5.2
- [0005](adr/0005-connectivity-ports-and-tunnel.md): подключение через проброс портов и туннель
- [0006](adr/0006-wiki-gm-only-no-crdt.md): вики только для мастера, без CRDT
- [0007](adr/0007-license-agpl-3.md): лицензия AGPL-3.0
- [0008](adr/0008-accounts-and-game-sessions.md): локальные аккаунты и игровые сессии вместо инвайтов

## Процесс
- [roles.md](process/roles.md): владелец, архитектор, исполнитель; статусы задач
- [workflow.md](process/workflow.md): от Issue до мерджа
- [brief-template.md](process/brief-template.md): шаблон брифа (промта) для исполнителя
- [review-checklist.md](process/review-checklist.md): чек-лист ревью реализации
- [conventions.md](process/conventions.md): соглашения по коду, коммитам, i18n
- [definition-of-done.md](process/definition-of-done.md): критерии готовности

## Статусы документов

В начале каждого документа в `systems/` и `architecture/` стоит строка статуса:
- `draft`: черновик, возможны большие изменения, код по нему писать только после уточнения;
- `review`: ждёт подтверждения пользователя;
- `stable`: утверждено, код должен соответствовать документу.
