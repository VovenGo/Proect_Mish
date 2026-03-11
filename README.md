# Мишаня шаманит

Веб-приложение на Go 1.25+ для домена `miha.vovengo.com`.
Тема: абсурдная киберпанк-деревня — неон по грязи, техномагия, тракторы-дроны, самовары с антеннами и прочий красивый деревенский угар.

## Что уже сделано

- Go backend без внешних фреймворков
- web UI с canvas-рисовалкой
- поле для текстового промпта
- загрузка референсной картинки
- API `POST /api/render`
- сохранение исходников и результата по job-id в `data/jobs`
- dockerized запуск через `Dockerfile` и `docker-compose.yml`
- два режима генерации:
  - `mock` — локальная заглушка, чтобы UI и пайплайн работали без секретов
  - `openai` — заготовка под OpenAI-compatible image provider

## Текущее состояние репозитория

На старте в репе был почти ноль: только `.git` и дефолтный `README.md`.
Локального `go` на хосте нет, поэтому проект специально собран так, чтобы жить через Docker.

## Архитектура

```text
cmd/server/main.go         entrypoint
internal/config            env-конфиг
internal/httpx             роуты + HTML/API
internal/service           orchestration/use-case слой
internal/gen               провайдеры генерации (mock/openai)
web/templates              HTML
web/static                 JS/CSS
```

Поток запроса:

1. Пользователь рисует на canvas и/или прикладывает reference image.
2. Фронт шлёт multipart на `/api/render`.
3. Backend сохраняет sketch/reference в job directory.
4. Сервис собирает финальный prompt из системной стилистики + пользовательского текста.
5. Дальше вызывается generator provider.
6. Готовый PNG сохраняется и раздаётся через `/assets/<job-id>/result.png`.

## Быстрый запуск

```bash
cp .env.example .env
docker compose up --build
```

После запуска открой:

- <http://localhost:8080>

## Переключение на реальную генерацию

По умолчанию стоит `GEN_PROVIDER=mock`.
Чтобы включить внешний image provider:

```bash
cp .env.example .env
# отредактируй .env
GEN_PROVIDER=openai
OPENAI_API_KEY=...
OPENAI_IMAGE_MODEL=gpt-image-1
OPENAI_BASE_URL=https://api.openai.com/v1
docker compose up --build
```

## Главный блокер / какие секреты нужны

Сейчас для production-генерации **не хватает валидного image generation provider**.
Нужен минимум один из вариантов:

- OpenAI / OpenAI-compatible API с доступом к image generation
- Replicate / Fal / Stability / другой провайдер (тогда надо дописать adapter в `internal/gen`)

### Что критично уточнить

- какой провайдер выбираем
- есть ли API key и лимиты
- умеет ли провайдер image-to-image / reference-guided generation
- нужен ли strict NSFW/safety policy
- нужно ли хранить пользовательские картинки долго или удалять по TTL

## Известные ограничения текущего scaffold

- `openai` adapter пока минимальный и рассчитан на совместимый `/images` API с `b64_json`
- нет очереди задач / async job processing
- нет auth
- нет persistent DB, только файловое хранилище job-артефактов
- нет reverse proxy/Caddy/Nginx для боевого TLS на `miha.vovengo.com`
- mock-режим не генерирует настоящую магию, только техничную заглушку

## Что я бы делал следующим шагом

1. Выбрать реального image provider.
2. Добавить нормальный provider adapter под его API.
3. Вынести jobs в SQLite/Postgres.
4. Сделать async статус job + history gallery.
5. Прикрутить Caddy/Traefik и боевой deploy под `miha.vovengo.com`.
6. Добавить инструменты редактирования: ластик, undo/redo, слои, mask/inpaint режим.

## Полезные env

- `HTTP_ADDR` — адрес приложения, по умолчанию `:8080`
- `PUBLIC_BASE_URL` — публичная база для ссылок
- `GEN_PROVIDER` — `mock` или `openai`
- `OPENAI_BASE_URL`
- `OPENAI_API_KEY`
- `OPENAI_IMAGE_MODEL`
- `OPENAI_IMAGE_SIZE`
- `APP_DOMAIN`
- `APP_NAME`

## Примечание

Скелет уже пригоден для локальной демонстрации и дальнейшей интеграции с настоящей генерацией. Для полного продукта нужна финализация выбора провайдера и боевого деплоя.
