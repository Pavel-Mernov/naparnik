# Контракт API

Фиксируем в первые часы и меняем только по договорённости в чате команды (владельцы: backend + frontend).
Все пути backend — с префиксом `/api/v1`. Авторизация — `Authorization: Bearer <token>` из `/auth/login`.
Время — ISO 8601 с таймзоной (`2026-11-10T09:30:00+03:00`). Ошибки — `{"detail": "текст по-русски", "field"?: "..."}`.

```
frontend ──/api/v1──► backend ──/v1──► ml (агент, RAG) ──► llm (Ollama)
                         │
                         ├── postgres
                         └── worker (напоминания) ──► in-app / Slack
```

Frontend ходит **только** в backend. Backend — единственный владелец БД и единственный, кто **выполняет действия**
(создать задачу, напоминание, отправить сообщение). ML-сервис stateless: получает контекст, возвращает текст,
цитаты и *предложенные* действия.

## Типы

```ts
type Role = "intern" | "mentor" | "employee" | "hr";

interface User { id: number; login: string; name: string; role: Role; team: string;
  title: string; avatar_color: string; mentor_id: number | null; start_date: string | null }

interface Task { id: number; title: string; status: "todo" | "doing" | "done";
  due_at: string | null; scheduled_at: string | null;          // scheduled_at — положили на таймлайн дня
  source: "manual" | "assistant" | "onboarding" }

interface Reminder { id: number; text: string; fire_at: string; channel: "app" | "slack";
  status: "pending" | "sent" | "seen" | "failed" }

interface CalendarEvent { id: number; title: string; starts_at: string; ends_at: string;
  kind: "meeting" | "speaker" | "training" | "one_on_one"; place: string | null;
  people: number[];                                            // user id участников
  prep: string | null }                                        // «2 вопроса, которые стоит задать»

interface ColleagueCard { user_id: number; name: string; title: string; team: string; avatar_color: string;
  filled: boolean;                                             // false — коллега карточку не заполнял
  address_form: "ty" | "vy" | null; preferred_name: string | null;
  preferred_channel: "slack" | "email" | "in_person" | null;
  response_hours: string | null;                               // «10:00–18:00, после 19 не пишите»
  available_from: number | null; available_to: number | null;  // часы для предупреждения «сейчас не пишите»
  can_help_with: string[]; notes_for_newcomers: string | null }

interface Citation { n: number; title: string; source: string; snippet: string }

interface AssistantAction { type: "create_task" | "set_reminder" | "remember";
  payload: Record<string, unknown>; result: { id: number } | null }   // result != null — backend уже выполнил

interface MemoryFact { id: number; text: string; kind: string; source: "chat" | "manual"; created_at: string }

interface OnboardingStep { id: number; week: number; title: string; description: string;
  kind: "access" | "meeting" | "training" | "task" | "people"; status: "todo" | "done" }
```

## Backend `/api/v1`

| Метод и путь | Тело → ответ | Зачем |
|---|---|---|
| `POST /auth/login` | `{login, password}` → `{token, user: User}` | Демо-вход, пароль у всех `demo` |
| `GET /me` | → `User` | |
| `GET /today` | → `{date, greeting, events: CalendarEvent[], tasks: Task[], reminders: Reminder[], term_of_day: {term, definition}}` | Утренний бриф = главный экран |
| `GET /tasks` · `POST /tasks` · `PATCH /tasks/{id}` · `DELETE /tasks/{id}` | `POST {title, due_at?, scheduled_at?}` · `PATCH` — любые поля Task | To-do и перетаскивание на таймлайн |
| `GET /reminders` · `POST /reminders` · `POST /reminders/{id}/seen` · `DELETE /reminders/{id}` | `POST {text, fire_at, channel?}` | Напоминания |
| `GET /notifications` | → `Reminder[]` со статусом `sent` (сработали, но не просмотрены) | Фронт опрашивает раз в 15 с |
| `GET /colleagues` | → `ColleagueCard[]` | Колода визиток |
| `GET /colleagues/{user_id}` | → `ColleagueCard` | |
| `PUT /me/card` | поля карточки → `ColleagueCard` | Сотрудник сам заполняет «как ко мне обращаться» (согласие) |
| `POST /colleagues/{user_id}/draft` | `{goal}` → `{text, tips: string[], warnings: string[], suggested_send_at: string \| null}` | «Как написать Ивану» |
| `POST /messages/send` | `{to_user_id, text}` → `{status: "sent" \| "demo", channel}` | Отправка **только по клику** человека |
| `POST /assistant/ask` | `{text, anonymous?: bool}` → `{answer, citations: Citation[], actions: AssistantAction[], confidence: "high" \| "low" \| "none", routed_to: {name, channel} \| null}` | Cmd+K и чат. `anonymous` — «глупый вопрос»: без ответа уходит менторам без имени |
| `GET /glossary` | → `{term, definition}[]` | Словарь жаргона компании |
| `GET /onboarding` | → `{title, week_now, steps: OnboardingStep[]}` | План первых недель |
| `POST /onboarding/steps/{id}/done` | → `OnboardingStep` | |
| `GET /memory` · `POST /memory` · `DELETE /memory/{id}` · `DELETE /memory` | `POST {text}` | «Что ты обо мне помнишь» + забыть |
| `POST /reports/weekly` | `{done, stuck, learn}` → `{id, draft}` | Пятничный самоотчёт → черновик к 1:1 |
| `GET /mentor/cohort` | → `{intern: User, progress_pct, stuck: string[], last_contact_days, questions_week}[]` | Доска ментора (только `mentor`/`hr`) |
| `GET /mentor/questions` | → `{top: {text, count}[], gaps: {text, count}[], anonymous: {id, text, created_at}[]}` | О чём спрашивают + пробелы в вики |
| `POST /admin/reset` | → `{ok: true}` | Сброс демо (только `hr`) |
| `GET /health` (без префикса) | → `{status: "ok"}` | |

## ML `/v1` (вызывает только backend)

| Метод и путь | Тело → ответ |
|---|---|
| `GET /health` | `{status, llm: {enabled, ready, model}, kb_docs}` |
| `POST /v1/respond` | `{text, user: {name, role, team}, memory: string[], now}` → `{answer, citations, actions: [{type, payload}], confidence, intent}` |
| `POST /v1/draft` | `{goal, sender: {name, role}, card: ColleagueCard, now}` → `{text, tips, warnings, suggested_send_at}` |
| `GET /v1/kb/search?q=&k=` | → `{hits: [{title, source, snippet, score}]}` |
| `GET /v1/glossary` | → `{term, definition}[]` |
| `POST /v1/weekly` | `{name, done, stuck, learn}` → `{draft}` |

Правило ML: без LLM (`OLLAMA_URL=` пусто) каждый эндпоинт отвечает правилами и шаблонами — демо не падает.
