# backend — FastAPI + PostgreSQL

**Владелец:** роль 1 (Backend) — [docs/team/roles.md](../docs/team/roles.md).

Единственный владелец БД. Отвечает за вход и роли, задачи, напоминания, карточки коллег, доску ментора,
**выполнение действий агента** и отправку сообщений (только по клику человека). Тексты не генерирует — это `ml`.
Контракт — [docs/api-contract.md](../docs/api-contract.md).

## Структура (план)

```
backend/
├── app/
│   ├── main.py            точка входа FastAPI, /health, подключение роутеров с префиксом /api/v1
│   ├── config.py          настройки из .env (pydantic-settings)
│   ├── api/               эндпоинты: auth, today, tasks, reminders, colleagues, assistant, onboarding,
│   │                      memory, reports, mentor, admin
│   ├── services/          логика: бриф дня, выполнение действий агента, доска ментора
│   ├── db/                модели (SQLModel), сессия, демо-данные (seed.py)
│   ├── migrations/        Alembic
│   ├── integrations/      адаптеры: slack.py, calendar.py (ICS), notion.py — у каждого демо-режим без токенов
│   ├── ml_client.py       клиент ml-сервиса; ml недоступен → честная заглушка
│   └── worker.py          напоминания: `python -m app.worker`, тот же образ
└── tests/                 pytest; SQLite для быстрых тестов, Postgres в CI
```

## Таблицы (план)

`user`, `colleague_card` (заполняет сам сотрудник, `consent_at`), `task`, `reminder`, `calendar_event`,
`onboarding_step`, `memory_fact`, `question` (анонимные — без автора), `weekly_report`, `sent_message`.

## Запуск

```bash
uv sync
uv run uvicorn app.main:app --reload --port 8000
uv run pytest
```

Зависимости уже выбраны в `pyproject.toml`, версии — в `uv.lock`.
