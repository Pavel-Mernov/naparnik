# Как работаем

- Роли и владельцы папок — [docs/team/roles.md](docs/team/roles.md).
- Ветки `feat/<кто>-<что>`, PR в `main`, CI зелёный, одно ревью владельца папки.
- Контракт [docs/api-contract.md](docs/api-contract.md) меняем только вдвоём (backend + frontend) и пишем в чат.
- Решения — в [docs/decisions.md](docs/decisions.md).
- Фронт не ждёт бэкенд (`VITE_MOCK=1`), бэкенд не ждёт ML (заглушка), ML не ждёт LLM (`OLLAMA_URL=` → шаблоны).
- Секреты только в `.env`, он в `.gitignore`.
