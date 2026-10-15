# Процесс разработки

Роли (владелец, архитектор, исполнитель) и статусы задач описаны в [roles.md](roles.md). Одновременно работает один агент.

## Жизненный цикл задачи

```
 Идея / баг (владелец)
     │
     ▼
 Issue status:needs-spec ──► архитектор: спека в docs/, владелец согласовал
     │
     ▼
 status:needs-brief ──► архитектор: бриф в теле Issue (brief-template.md)
     │
     ▼
 status:ready ──► исполнитель: ветка → код → тесты → PR (Closes #N)
     │
     ▼
 status:in-review ──► архитектор: ревью (review-checklist.md)
     │                     │
     │ APPROVE             │ CHANGES → status:changes-requested → исполнитель правит
     ▼
 владелец: squash merge в main
```

### 1. Подготовка сессии (любая роль)
```bash
git switch main
git pull --ff-only
git status            # должно быть чисто
git branch -a         # нет ли незавершённых веток
gh pr list            # нет ли открытых PR
```

### 2. Выбор задачи (архитектор)
- Текущий майлстоун — первый незакрытый в [roadmap.md](../roadmap.md).
- Приоритет: `prio:p0` → `p1` → `p2`, затем по номеру.
- Задача больше ~400 строк изменений → разбить на несколько Issues, каждая со своим брифом.

### 3. Спецификация (архитектор)
- Поведение описано в `docs/systems/*` или `docs/architecture/*`? Если нет или неоднозначно — дописать документ и согласовать с владельцем **до** брифа.
- Архитектурное решение (новая зависимость, новый протокол, смена хранилища) → новый ADR (`docs/adr/NNNN-*.md`, статус `proposed`). Владелец переводит его в `accepted`.
- Изменения документации идут отдельным PR `docs/...` и мерджатся до брифа, чтобы исполнитель читал актуальный `main`.

### 4. Бриф (архитектор)
- По [brief-template.md](brief-template.md), в тело Issue: `gh issue edit <N> --body-file brief.md`.
- Метки: снять `status:needs-brief`, поставить `status:ready`.

### 5. Реализация (исполнитель)
- Ветка из брифа. Маленькие коммиты, каждый собирается.
- Тесты вместе с кодом (см. [conventions.md](conventions.md#тесты)).
- Новые строки UI — только через i18n, сразу `ru` и `en`.
- PR по шаблону: `Closes #N`, «Отклонения от брифа». Issue → `status:in-review`.

### 6. Ревью (архитектор)
- По [review-checklist.md](review-checklist.md). Вердикт — комментарием в PR.
- APPROVE → снять статусную метку, сообщить владельцу. CHANGES → `status:changes-requested`.

### 7. После мерджа (владелец или архитектор)
- Отметить пункт в [roadmap.md](../roadmap.md), если закрыт крупный блок.
- Удалить локальную ветку: `git fetch --prune`, `git branch -D <branch>`.

## Метки (labels)

| Группа | Значения |
|--------|----------|
| type | `type:feature`, `type:bug`, `type:docs`, `type:chore`, `type:spike` |
| area | `area:server`, `area:client`, `area:vtt`, `area:av`, `area:sheet`, `area:wiki`, `area:net`, `area:build` |
| status | `status:needs-spec`, `status:needs-brief`, `status:ready`, `status:in-review`, `status:changes-requested`, `status:blocked` |
| prio | `prio:p0` (блокер), `prio:p1` (важно), `prio:p2` (можно позже) |

Создаются скриптом [`scripts/github-bootstrap.ps1`](../../scripts/github-bootstrap.ps1):

```powershell
powershell -ExecutionPolicy Bypass -File scripts/github-bootstrap.ps1
```

Служебные скрипты пишутся на PowerShell: основная платформа — Windows, а Git Bash/WSL не видят Windows-версию `gh` и падают при нехватке памяти.

## Spike-задачи

Исследование с неизвестным исходом (например, туннель + WebRTC). Правила:
- Ограничено по времени (указывается в Issue).
- Код spike живёт в `spikes/<name>/` или в отдельной ветке и **не** мерджится в основной код как есть.
- Результат — отчёт в Issue и обновлённый или новый ADR.

## Работа без GitHub

Если `gh` недоступен, задачу ставит пользователь в чате; ветки и коммиты ведутся так же, а вместо PR агент показывает `git diff main...HEAD` и ждёт команды на мердж.
