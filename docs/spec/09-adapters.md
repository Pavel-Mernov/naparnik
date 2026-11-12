# 09. Адаптеры сервисов, выгрузка событий, feature flags

Код: [`internal/adapters`](../../backend/internal/adapters), [`internal/flags`](../../backend/internal/flags), выбор реализаций — [`app.Services`](../../backend/internal/app/app.go). Продуктовый контракт интеграций — [11-INTEGRATION](../03-requirements/11-INTEGRATION.md).

## Как подключить настоящий сервис Авито

Инструменты агента не знают, мок за интерфейсом или настоящая система. Подключение — новая реализация интерфейса и строка в `app.Services`; агент, промпт, права (BR-40), черновики и подтверждение (BR-43), квитанции и сбои (SC-15) не меняются.

```
агент ──► tools (права, черновик, квитанция) ──► adapters.Set ──┬─► Mock*           (docs/04-mock-data)
                                                               └─► HTTPAbsence     (ABSENCE_SERVICE_URL)
                                                               └─► … ваша реализация
```

| Интерфейс | Методы | Сейчас | В Авито |
|---|---|---|---|
| `AbsenceService` | `Catalog` — типы, порядок «сделай сам», коды отказа; `Submit` — заявка → квитанция | `MockAbsence`, `HTTPAbsence` | Система отсутствий |
| `AccessService` | `Catalog` — системы и коды отказа; `Granted` — что уже выдано; `Submit` | `MockAccess` | Заявки на доступ (Jira) |
| `Calendar` | `Events`, `Busy(user)` | `MockCalendar` | Календарь |
| `KnowledgeBase` | `Pages`, `AddPage`, `UpdatePage` | `store.Store` (PostgreSQL) | Вики команды |
| `Tracker` | `PlanSteps`, `SetStepStatus`, `TemplateStep` | `store.Store` (PostgreSQL) | Трекер (эпик плана, шаги-задачи) |
| `store.EventSink` | пачки событий журнала | `Exporter` → `HTTPCollector` / `JSONLines` | Clickstream / Kafka |
| `flags.Provider` | `Enabled(name, roles)` | `FEATURE_FLAGS`, `FLAGS_FILE` | Runtime feature flags |

Для `KnowledgeBase` и `Tracker` хранилище реализует интерфейс — проверяется при компиляции (`tools/services.go`).

Авторизация и роли не дублируются: пользователь приходит из SSO Авито (в прототипе — заголовок `X-User`), роли — из ролевой модели (`team.json`).

## Правила для любой реализации

| Правило | MUST |
|---|---|
| Сбой сети, 5xx, 429 → `adapters.ErrUnavailable`: инструмент отвечает `unavailable`, черновик сохраняется, «Повторить» безопасно | MUST, `TestHTTPAbsence`, `TestToolsUseConnectedService` |
| `Submit` идемпотентен по `idempotency_key` (`abs:<сотрудник>:<тип>:<дата>`, `acc:<сотрудник>:<система>`): повтор → та же квитанция | MUST |
| Номер квитанции — из сервиса (`Receipt.ID`); Напарник хранит его и показывает стажёру | MUST |
| Права проверяет Напарник до вызова сервиса; сервис вправе проверить ещё раз | MUST |
| В заявку не передаются причина отсутствия, диагноз и другие ПДн (BR-61) | MUST |
| Справочник можно кэшировать; при сбое отдаётся последний известный | SHOULD |

## HTTP-контракт сервиса отсутствий (`HTTPAbsence`)

`ABSENCE_SERVICE_URL` — адрес, `ABSENCE_SERVICE_TOKEN` — сервисный токен (`Authorization: Bearer`). Таймаут 5 с, справочник в кэше 10 минут.

| Запрос | Тело | Ответ |
|---|---|---|
| `GET /catalog` | — | `{request_types: [{type, title, min_notice_workdays…}], self_service: {title, steps, link, source_page}, reason_codes: [{code, text}]}` |
| `POST /requests`, `Idempotency-Key` | `{idempotency_key, employee_id, approver_id, type, date, comment?}` | `{id, status}` — `pending` по умолчанию |

Статус после согласования (`approved`, `rejected`) в прототипе меняет экран руководителя; с настоящим сервисом — его вебхук или опрос (следующий шаг).

## Выгрузка журнала событий

`store.Emit` пишет событие в журнал (PostgreSQL); после коммита транзакции событие передаётся в `store.EventSink`. Откатившееся событие наружу не уходит. Тексты вопросов, ответов и причины отсутствия в событиях не хранятся (BR-51, BR-72).

| | |
|---|---|
| `EVENTS_URL` | HTTP-коллектор: `POST {"events": [ClickEvent…]}` пачками до 100, раз в 2 с; `EVENTS_TOKEN` — `Bearer` |
| `EVENTS_FILE` | JSONL-файл (для сборщика логов или локальной проверки) |
| `ClickEvent` | `event_id`, `ts`, `source: naparnik`, `event_type`, `actor_id`, `object_id`, `meta` |
| Сбой коллектора | Пачка повторяется со следующей отправкой; работа Напарника не блокируется |
| Переполнение | Буфер 10 000; лишнее отбрасывается со счётчиком; журнал в PostgreSQL — источник правды |
| Наблюдаемость | `GET /api/health` → `events: {sent, dropped, failed_batches, queued}` |

Тесты: `TestEventsExportAfterCommit`, `TestEventWriters`.

## Feature flags

| Флаг | Что выключает |
|---|---|
| `practice` | Инструменты практики перед задачей (`get_practice`, `advance_practice`) |
| `glossary` | Жаргон и набор знаний роли (`explain_term`, `suggest_term`, `get_role_knowledge`) |
| `meetings` | Назначение встреч по вопросам |
| `telegram` | Доставку в Telegram (уведомления копятся в вебе и уйдут после включения) |
| `watchers` | Сторожа сроков: SLA, простой, автоактивация плана |

- `FEATURE_FLAGS=practice=off,telegram=on` — при запуске. `FLAGS_FILE` — JSON `{"practice": {"on": true, "roles": ["intern"]}}`, перечитывается раз в 5 с без перезапуска; файл важнее переменной.
- Флаг без значения включён. `roles` — постепенное включение по ролям.
- Выключенный инструмент не передаётся модели, не виден в MCP и не выполняется — проверка в `tools.Allowed`.
- Текущие значения — `GET /api/health` → `flags`.

Тесты: `TestFlags`, `TestToolsRespectFlags`, `TestTelegramFlagOff`.
