# Creates labels, milestones and the initial M0 issues in the current GitHub repo.
# Run once from the repo root after `gh auth login`:
#   powershell -ExecutionPolicy Bypass -File scripts/github-bootstrap.ps1
# Works in Windows PowerShell 5.1 and PowerShell 7. Safe to re-run: labels are
# upserted, milestones and issues are created only if missing.
# The file is saved as UTF-8 with BOM so Windows PowerShell 5.1 reads Cyrillic correctly.
$ErrorActionPreference = 'Stop'
# gh prints UTF-8; without this PowerShell 5.1 mis-decodes titles and the "exists" checks miss.
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

if (-not (Get-Command gh -ErrorAction SilentlyContinue)) {
    throw 'gh CLI is required: https://cli.github.com'
}

function Invoke-Gh {
    & gh @args
    if ($LASTEXITCODE -ne 0) { throw "gh $($args -join ' ') failed with exit code $LASTEXITCODE" }
}

$repo = Invoke-Gh repo view --json nameWithOwner -q .nameWithOwner
Write-Host "Bootstrapping $repo"

$labels = @(
    @('type:feature',      '1d76db', 'Новая функциональность'),
    @('type:bug',          'd73a4a', 'Ошибка'),
    @('type:docs',         '0075ca', 'Документация'),
    @('type:chore',        'cfd3d7', 'Инфраструктура, зависимости'),
    @('type:spike',        '5319e7', 'Исследование, ограниченное по времени'),
    @('area:server',       'c5def5', 'Go-сервер'),
    @('area:client',       'c5def5', 'Веб-клиент'),
    @('area:vtt',          'bfdadc', 'Виртуальный стол'),
    @('area:av',           'bfdadc', 'Голос и видео'),
    @('area:sheet',        'bfdadc', 'Лист персонажа'),
    @('area:wiki',         'bfdadc', 'Вики мастера'),
    @('area:net',          'bfdadc', 'Сеть, туннель, порты'),
    @('area:build',        'bfdadc', 'Сборка, CI, упаковка'),
    @('status:needs-spec', 'fbca04', 'Нужна спецификация или ответ пользователя'),
    @('status:ready',      '0e8a16', 'Можно брать в работу'),
    @('status:blocked',    'b60205', 'Заблокировано'),
    @('prio:p0',           'b60205', 'Блокер'),
    @('prio:p1',           'd93f0b', 'Важно'),
    @('prio:p2',           'fef2c0', 'Можно позже')
)
foreach ($l in $labels) {
    Invoke-Gh label create $l[0] --color $l[1] --description $l[2] --force | Out-Null
    Write-Host "label $($l[0])"
}

$milestones = [ordered]@{
    'M0 Foundation'             = 'Скелет приложения, вход по ссылке, CI, spike по WebRTC и туннелю'
    'M1 VTT core'               = 'Сцены, фишки, сетка, туман, кубики, чат, инициатива'
    'M2 Voice & Video'          = 'Встроенный SFU, камеры, simulcast, TURN'
    'M3 GM Wiki'                = 'Страницы, базы данных, упоминания, handout''ы'
    'M4 Character Sheet'        = 'Лист персонажа по SRD 5.2'
    'M5 Networking & Packaging' = 'Туннель одной кнопкой, упаковка, бэкапы'
}
$existingMs = @(Invoke-Gh api "repos/$repo/milestones?state=all&per_page=100" -q '.[].title')
foreach ($title in $milestones.Keys) {
    if ($existingMs -contains $title) { Write-Host "milestone $title (exists)"; continue }
    Invoke-Gh api "repos/$repo/milestones" -f "title=$title" -f "description=$($milestones[$title])" | Out-Null
    Write-Host "milestone $title"
}

$issues = @(
    @{ title = 'Server skeleton: cmd/dndvtt, config, slog, graceful shutdown'
       labels = 'type:feature,area:server,status:ready,prio:p0'
       body = @'
Спека: docs/architecture/overview.md (структура кода, жизненный цикл).

- [ ] go.mod, cmd/dndvtt/main.go, internal/app, internal/config
- [ ] Флаги и конфиг: порты, папка данных, dev-режим
- [ ] log/slog, graceful shutdown по Ctrl+C
- [ ] Healthcheck GET /api/health
'@ },
    @{ title = 'Client skeleton: Vite + React + TS + i18next, embedded via go:embed'
       labels = 'type:feature,area:client,area:build,status:ready,prio:p0'
       body = @'
Спека: docs/architecture/overview.md, docs/process/conventions.md.

- [ ] web/ на pnpm + Vite + React + TS strict, ESLint, Prettier, Vitest
- [ ] i18next с ru/en и тестом на полноту ключей
- [ ] web/embed.go и раздача SPA сервером (fallback на index.html)
- [ ] dev-режим: Vite dev server с прокси на Go
'@ },
    @{ title = 'SQLite store and migrations'
       labels = 'type:feature,area:server,status:ready,prio:p0'
       body = @'
Спека: docs/architecture/data-model.md, ADR-0003.

- [ ] modernc.org/sqlite, WAL, foreign_keys
- [ ] Раннер встроенных миграций + тесты
- [ ] Первая миграция: campaign, player, invite
'@ },
    @{ title = 'WebSocket hub and base protocol (hello/snapshot/presence/ping)'
       labels = 'type:feature,area:server,area:client,status:ready,prio:p0'
       body = @'
Спека: docs/architecture/protocol.md.

- [ ] /ws, конверт сообщений, ack/error
- [ ] session.hello / session.snapshot, seq и буфер для переподключения
- [ ] presence.update, keepalive
- [ ] Решить: генерация TS-типов из Go (tygo) или тест-сверка
- [ ] Интеграционный тест на httptest
'@ },
    @{ title = 'Invites, GM/Player roles and player sessions'
       labels = 'type:feature,area:server,status:ready,prio:p0'
       body = @'
Спека: docs/architecture/security.md.

- [ ] GM-вход по одноразовому токену с loopback
- [ ] Создание и отзыв инвайтов (хеш токена, срок, лимит)
- [ ] Страница /join/<token>: ввод имени, cookie сессии
- [ ] Middleware ролей, тесты прав
'@ },
    @{ title = 'CI: lint, test, build for Windows and Linux'
       labels = 'type:chore,area:build,status:ready,prio:p1'
       body = @'
- [ ] GitHub Actions: Go (vet, golangci-lint, test), web (lint, test, build)
- [ ] Сборка артефактов windows/amd64 и linux/amd64 с встроенным клиентом
- [ ] Markdown-lint и проверка ссылок в docs/
'@ },
    @{ title = 'Spike: pion SFU with 3 cameras over LAN'
       labels = 'type:spike,area:av,status:ready,prio:p1'
       body = @'
Вопрос: реально ли быстро поднять SFU на pion v4 с simulcast для 3+ участников, или брать LiveKit sidecar (ADR-0002).
Ограничение: 1–2 дня.

- [ ] Прототип: комната, 3 браузера, камеры и звук
- [ ] Замеры CPU и трафика
- [ ] Вывод и обновление ADR-0002
'@ },
    @{ title = 'Spike: external access — port forwarding, HTTPS and tunnel with media'
       labels = 'type:spike,area:net,status:ready,prio:p1'
       body = @'
Вопрос: см. ADR-0005 и docs/architecture/networking.md (варианты A/B/C, HTTPS при пробросе).
Ограничение: 2 дня.

- [ ] Проверить WebRTC через playit.gg (UDP)
- [ ] Проверить TURN-over-TCP через HTTP-туннель (Cloudflare)
- [ ] Варианты HTTPS при пробросе портов
- [ ] Вывод: обновить ADR-0005 до accepted
'@ }
)

$existingIssues = @(Invoke-Gh issue list --state all --limit 500 --json title -q '.[].title')
# Bodies go through a UTF-8 file: passing multi-line Cyrillic as an argument is fragile in PowerShell 5.1.
$bodyFile = [System.IO.Path]::GetTempFileName()
try {
    foreach ($i in $issues) {
        if ($existingIssues -contains $i.title) { Write-Host "issue '$($i.title)' (exists)"; continue }
        [System.IO.File]::WriteAllText($bodyFile, $i.body, (New-Object System.Text.UTF8Encoding $false))
        Invoke-Gh issue create --title $i.title --label $i.labels --milestone 'M0 Foundation' --body-file $bodyFile | Out-Null
        Write-Host "issue '$($i.title)'"
    }
} finally {
    Remove-Item $bodyFile -ErrorAction SilentlyContinue
}

Write-Host 'Done.'
