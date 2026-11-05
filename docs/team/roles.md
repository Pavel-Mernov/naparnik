# Команда из 5 человек: кто чем владеет

Правило: у каждой папки один владелец. Чужие файлы правим через PR с ревью владельца. Контракт
[`docs/api-contract.md`](../api-contract.md) меняем только вдвоём (backend + frontend) и пишем об этом в чат.

| Роль | Владеет | Первые задачи |
|---|---|---|
| **1. Backend** | `backend/`, `docker-compose.yml`, `scripts/smoke_api.py` | Alembic вместо `create_all`; worker на Postgres (`SKIP LOCKED`); бриф в 9:00; реальный Slack-адаптер (Socket Mode для DM) |
| **2. ML / LLM** | `ml/`, `llm/` | Замер моделей (`ml/eval`): роутер ≥ 85%, RAG hit@3 ≥ 80%; эмбеддинги `bge-m3` + перенос индекса в pgvector; импорт Notion → `ml/data/kb` |
| **3. Frontend: «Сегодня» и анимации** | `frontend/src/pages/Today*`, `components/timeline/*`, токены в `styles.css` | Довести бриф-анимацию, drag-n-drop, утренний сценарий до «вау»; проверить на проекторе |
| **4. Frontend: Люди, Cmd+K, Ментор** | `frontend/src/pages/People*`, `Mentor*`, `components/palette/*` | Визитка-переворот → черновик → отправка; доска ментора; режим анонимного вопроса |
| **5. Продукт / аналитик / питч** | `docs/`, демо-данные (`backend/app/seed.py`, `ml/data/kb/*.md`, `ml/data/glossary.yaml`) | Реалистичная демо-вики (15–30 страниц), глоссарий, вопросы для eval, демо-сценарий, презентация, видео-бэкап |

## Как работаем

- Ветки: `feat/<кто>-<что>`, PR в `main`, CI зелёный, одно ревью.
- Фронт не ждёт бэкенд: `VITE_MOCK=1 npm run dev` — экраны на фикстурах по контракту.
- Бэкенд не ждёт ML: при недоступном ml backend отвечает заглушкой; ML не ждёт LLM: `OLLAMA_URL=` → шаблоны.
- Каждый вечер: `docker compose up --build` + `python3 scripts/smoke_api.py` на `main`.
- Решения — в [`docs/decisions.md`](../decisions.md).
