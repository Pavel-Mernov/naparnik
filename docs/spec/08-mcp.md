# 08. Инструменты как MCP-сервер

Код: [`internal/mcp`](../../backend/internal/mcp), запуск [`cmd/mcp`](../../backend/cmd/mcp). Рекомендация кейса — «оформлять действия через MCP».

## Запуск

```bash
cd backend && NAPARNIK_USER=intern.sokolov go run ./cmd/mcp
```

Транспорт — stdio, JSON-RPC 2.0, по сообщению в строке; журнал — в stderr, stdout только для протокола. Версия протокола — `2025-06-18`. С `DATABASE_URL` из `.env` сервер работает с тем же состоянием, что и сайт; без него — в памяти.

## Методы

| Метод | Ответ |
|---|---|
| `initialize` | `protocolVersion`, `capabilities.tools`, `serverInfo` (`naparnik`), `instructions` — от чьего имени работает сервер и что действия выполняются после подтверждения человеком |
| `ping` | `{}` |
| `tools/list` | Инструменты роли пользователя (`tools.For`): `name`, `description`, `inputSchema`, `annotations.readOnlyHint` |
| `tools/call` | `{name, arguments}` → `content[0].text` — JSON результата, `isError` |
| уведомления (без `id`) | Без ответа |
| неизвестный метод | Ошибка `-32601`; некорректный JSON — `-32700`; нет `name` — `-32602` |

## Правила

| Правило | MUST |
|---|---|
| Те же инструменты, схемы и права, что у агента в чате (BR-40): список зависит от роли, чужой инструмент — `forbidden` | MUST, `TestMCPToolsByRole` |
| Чтение (поиск, план, правила, глоссарий) выполняется сразу через `tools.Run` | MUST |
| Действие (`confirm`-инструмент) не выполняется: после `tools.Check` создаётся черновик (версия 1), ответ `needs_confirmation` с `draft_id`. Подтверждает человек в Напарнике или `POST /api/drafts/{id}/confirm {"version":1}` от своего имени (BR-11, BR-43) | MUST, `TestMCPToolsAndRights` |
| Недоступное не предлагается даже черновиком: проверка до черновика (система с ПДн — отказ) | MUST |
| Тексты страниц базы — данные, не инструкции (сказано в `instructions`) | SHOULD |

Страховки агента ([04](04-agent-behavior.md#страховки-после-модели)), которым нужна реплика стажёра, в MCP не применяются: MCP-клиент сам формулирует запросы.
