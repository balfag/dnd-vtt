#!/usr/bin/env bash
# Creates labels, milestones and the initial M0 issues in the current GitHub repo.
# Run it once from the repo root after `gh auth login` and after the repo has a GitHub remote.
# Labels are upserted (--force); milestones and issues are created only if missing.
set -euo pipefail

# On Windows `bash` may be WSL or Git Bash, which do not see the Windows gh on PATH.
GH=""
for c in gh gh.exe "/c/Program Files/GitHub CLI/gh.exe" "/mnt/c/Program Files/GitHub CLI/gh.exe"; do
  if command -v "$c" >/dev/null 2>&1 || [ -x "$c" ]; then GH="$c"; break; fi
done
[ -n "$GH" ] || { echo "gh CLI is required: https://cli.github.com" >&2; exit 1; }
# gh.exe called from WSL prints CRLF; strip CR so titles compare exactly.
gh() { "$GH" "$@" | tr -d '\r'; }

repo=$(gh repo view --json nameWithOwner -q .nameWithOwner)
echo "Bootstrapping $repo"

label() { gh label create "$1" --color "$2" --description "$3" --force >/dev/null; echo "label $1"; }

label "type:feature"      "1d76db" "Новая функциональность"
label "type:bug"          "d73a4a" "Ошибка"
label "type:docs"         "0075ca" "Документация"
label "type:chore"        "cfd3d7" "Инфраструктура, зависимости"
label "type:spike"        "5319e7" "Исследование, ограниченное по времени"
label "area:server"       "c5def5" "Go-сервер"
label "area:client"       "c5def5" "Веб-клиент"
label "area:vtt"          "bfdadc" "Виртуальный стол"
label "area:av"           "bfdadc" "Голос и видео"
label "area:sheet"        "bfdadc" "Лист персонажа"
label "area:wiki"         "bfdadc" "Вики мастера"
label "area:net"          "bfdadc" "Сеть, туннель, порты"
label "area:build"        "bfdadc" "Сборка, CI, упаковка"
label "status:needs-spec" "fbca04" "Нужна спецификация или ответ пользователя"
label "status:ready"      "0e8a16" "Можно брать в работу"
label "status:blocked"    "b60205" "Заблокировано"
label "prio:p0"           "b60205" "Блокер"
label "prio:p1"           "d93f0b" "Важно"
label "prio:p2"           "fef2c0" "Можно позже"

existing_ms=$(gh api "repos/$repo/milestones?state=all&per_page=100" -q '.[].title')
milestone() {
  if grep -qxF "$1" <<<"$existing_ms"; then echo "milestone $1 (exists)"; return; fi
  gh api "repos/$repo/milestones" -f title="$1" -f description="$2" >/dev/null
  echo "milestone $1"
}
milestone "M0 Foundation"           "Скелет приложения, вход по ссылке, CI, spike по WebRTC и туннелю"
milestone "M1 VTT core"             "Сцены, фишки, сетка, туман, кубики, чат, инициатива"
milestone "M2 Voice & Video"        "Встроенный SFU, камеры, simulcast, TURN"
milestone "M3 GM Wiki"              "Страницы, базы данных, упоминания, handout'ы"
milestone "M4 Character Sheet"      "Лист персонажа по SRD 5.2"
milestone "M5 Networking & Packaging" "Туннель одной кнопкой, упаковка, бэкапы"

existing_issues=$(gh issue list --state all --limit 500 --json title -q '.[].title')
issue() { # title labels body
  if grep -qxF "$1" <<<"$existing_issues"; then echo "issue '$1' (exists)"; return; fi
  gh issue create --title "$1" --label "$2" --milestone "M0 Foundation" --body "$3" >/dev/null
  echo "issue '$1'"
}

issue "Server skeleton: cmd/dndvtt, config, slog, graceful shutdown" "type:feature,area:server,status:ready,prio:p0" \
"Спека: docs/architecture/overview.md (структура кода, жизненный цикл).

- [ ] go.mod, cmd/dndvtt/main.go, internal/app, internal/config
- [ ] Флаги и конфиг: порты, папка данных, dev-режим
- [ ] log/slog, graceful shutdown по Ctrl+C
- [ ] Healthcheck GET /api/health"

issue "Client skeleton: Vite + React + TS + i18next, embedded via go:embed" "type:feature,area:client,area:build,status:ready,prio:p0" \
"Спека: docs/architecture/overview.md, docs/process/conventions.md.

- [ ] web/ на pnpm + Vite + React + TS strict, ESLint, Prettier, Vitest
- [ ] i18next с ru/en и тестом на полноту ключей
- [ ] web/embed.go и раздача SPA сервером (fallback на index.html)
- [ ] dev-режим: Vite dev server с прокси на Go"

issue "SQLite store and migrations" "type:feature,area:server,status:ready,prio:p0" \
"Спека: docs/architecture/data-model.md, ADR-0003.

- [ ] modernc.org/sqlite, WAL, foreign_keys
- [ ] Раннер встроенных миграций + тесты
- [ ] Первая миграция: campaign, player, invite"

issue "WebSocket hub and base protocol (hello/snapshot/presence/ping)" "type:feature,area:server,area:client,status:ready,prio:p0" \
"Спека: docs/architecture/protocol.md.

- [ ] /ws, конверт сообщений, ack/error
- [ ] session.hello / session.snapshot, seq и буфер для переподключения
- [ ] presence.update, keepalive
- [ ] Решить: генерация TS-типов из Go (tygo) или тест-сверка
- [ ] Интеграционный тест на httptest"

issue "Invites, GM/Player roles and player sessions" "type:feature,area:server,status:ready,prio:p0" \
"Спека: docs/architecture/security.md.

- [ ] GM-вход по одноразовому токену с loopback
- [ ] Создание и отзыв инвайтов (хеш токена, срок, лимит)
- [ ] Страница /join/<token>: ввод имени, cookie сессии
- [ ] Middleware ролей, тесты прав"

issue "CI: lint, test, build for Windows and Linux" "type:chore,area:build,status:ready,prio:p1" \
"- [ ] GitHub Actions: Go (vet, golangci-lint, test), web (lint, test, build)
- [ ] Сборка артефактов windows/amd64 и linux/amd64 с встроенным клиентом
- [ ] Markdown-lint и проверка ссылок в docs/"

issue "Spike: pion SFU with 3 cameras over LAN" "type:spike,area:av,status:ready,prio:p1" \
"Вопрос: реально ли быстро поднять SFU на pion v4 с simulcast для 3+ участников, или брать LiveKit sidecar (ADR-0002).
Ограничение: 1–2 дня.

- [ ] Прототип: комната, 3 браузера, камеры и звук
- [ ] Замеры CPU и трафика
- [ ] Вывод и обновление ADR-0002"

issue "Spike: external access — port forwarding, HTTPS and tunnel with media" "type:spike,area:net,status:ready,prio:p1" \
"Вопрос: см. ADR-0005 и docs/architecture/networking.md (варианты A/B/C, HTTPS при пробросе).
Ограничение: 2 дня.

- [ ] Проверить WebRTC через playit.gg (UDP)
- [ ] Проверить TURN-over-TCP через HTTP-туннель (Cloudflare)
- [ ] Варианты HTTPS при пробросе портов
- [ ] Вывод: обновить ADR-0005 до accepted"

echo "Done."
