# 04. Мок-данные

> Синтетические данные и заглушки интеграций для всех сценариев из [06-TEST-PERSONAS.md](../03-requirements/06-TEST-PERSONAS.md). Требование кейса: «тестовые данные и имитации интеграций, необходимые для воспроизведения сценариев».
> Все люди, отдел и тексты выдуманы (BR-63). Тестовые инструкции помечены `test_instruction: true` и показываются с пометкой «тестовая инструкция, не регламент Авито» (BR-05).
> Демо-дата: **2026-11-11** (среда, 3-й день стажировки Даниила). Сценарии со сдвигом времени описаны в `scenario_variants` и `sla_variant`.

**Проверка целостности:** `python3 docs/04-mock-data/check_mocks.py` — валидность JSON, ссылки на людей, страницы, шаги и встречи, покрытие всех сценариев SC-01…SC-29.

## Файлы

| Файл | Что внутри | Заменяет в проде |
|---|---|---|
| [team.json](./team.json) | Отдел, 7 человек, роли, рабочие часы, настройки стажёров | Данные сотрудников и ролевая модель Авито |
| [adaptation-plan-template.json](./adaptation-plan-template.json) | Шаблон плана: 5 этапов, 20 шагов по листу «Материалы для всех» чек-листа Авито + демо-шаги аналитика | Шаблон отдела |
| [tracker.json](./tracker.json) | Эпики адаптации: план Даниила со статусами, черновик плана Полины | Jira |
| [calendar.json](./calendar.json) | События демо-недели, занятость, ожидаемые слоты встречи | Корпоративный календарь |
| [kb-pages.json](./kb-pages.json) | 2 пространства, 7 папок, 20 страниц; пробелы под вопросы, дубли, «не дубль», закрытое пространство, открытый вопрос, неподтверждённый черновик | Confluence |
| [meetings.json](./meetings.json) | Встреча с записью (MTG-301) и без записи (MTG-302), напоминания включить запись, ожидаемая выжимка | ВКС и запись |
| [transcripts/MTG-301.md](./transcripts/MTG-301.md) | Транскрипт живой речью с таймкодами и посторонними темами | Распознавание речи |
| [questions.json](./questions.json) | Вопросы Q1–Q14 с ожидаемой веткой, адресатом и причиной выбора | — |
| [expert-answers.json](./expert-answers.json) | Ответы людей: текст, «не ко мне», переадресация бадди, встреча, отказ отвечать текстом; вариант с истёкшим SLA | Живые люди |
| [absence-service.json](./absence-service.json) | Сервис отсутствий: типы, «сделай сам», предпроверка, ответы 201/503/422, смена статуса, коды причин | Сервис отсутствий Авито |
| [access-service.json](./access-service.json) | Сервис доступов: системы по неделям, текущие доступы, ответы 201/403/503 | Сервис заявок на доступ |
| [notifications.json](./notifications.json) | Шаблоны уведомлений веб и Telegram, привязка бота, таймеры маскота | Корпоративный мессенджер |
| [evals.json](./evals.json) | Эталонный набор: 42 вопроса в 7 категориях для метрик M-14…M-17 | — |
| [llm-stub.json](./llm-stub.json) | Переменные окружения LLM и режимы заглушки: ок, медленно, таймаут, ошибка | Внешний LLM |

## Покрытие требований кейса Авито

| Требование кейса | Сценарии | Данные |
|---|---|---|
| **01.** Путь стажёра в первые недели по чек-листу Авито | SC-01, SC-18, SC-20 | adaptation-plan-template (шаги `avito-checklist:all`), tracker, calendar |
| **02.** Ответы о команде и инструментах с учётом контекста | SC-02, SC-03, SC-04, SC-07, SC-08, SC-12 | kb-pages, team, questions Q1–Q6, expert-answers |
| **03.** Действие через корпоративный сервис: результат показан и сохранён | SC-14, SC-15, SC-27, SC-29 | absence-service, access-service (квитанции ABS-1042, ACC-2002) |
| **04.** Этапы адаптации, статусы задач, следующие шаги | SC-01, SC-18, SC-19, SC-21 | tracker, adaptation-plan-template |
| **05.** Права и подключение сервисов | SC-09, SC-13, SC-16 | kb-pages (space-dwh-pii), access-service (403), team (настройки видимости), evals (forbidden) |
| **06.** Метрики пользы и проверка эффекта | SC-21 + прогон evals | evals (M-14…M-17), tracker (`first_useful_task_candidate`) |
| Огр. 02–03: синтетика, тестовые инструкции не выдаются за регламенты | SC-14, SC-27 | kb-02, kb-11 с `test_instruction: true` |
| Огр. 05: недоступное не показывается и не выполняется | SC-16, SC-13 | kb-R1, sys-dwh-pii, evals e-26…e-31 |
| Огр. 06: без чувствительных ПДн и финансов | SC-17, SC-28 | questions Q8, Q13 (заведомо фиктивные значения), evals e-32…e-37 |
| Огр. 09–10: < 1 с, LLM не блокирует интерфейс, сбои обработаны | SC-15, SC-26 | llm-stub, absence-service (503) |
| Сдача 03: тестовые данные и имитации интеграций | все SC-01…SC-29 | вся папка + check_mocks.py |
| «Наставник: сокращать рутину, не заменять содержательную работу» | SC-04, SC-10, SC-19, SC-21, SC-22, SC-23 | expert-answers, meetings, kb-pages (maintenance_seed) |

**Не покрывается данными, а делается в коде:** покрытие тестами > 80 % и отчёт о покрытии, SDD-спецификации агента и инструментов, README с инструкцией запуска.

## Сценарии → данные

| Сценарий | Данные |
|---|---|
| SC-01 Утро стажёра | tracker (s-01, s-03 Today; s-04 Overdue), calendar ev-02, notifications |
| SC-02 Знакомство с командой | team, kb-01, шаг s-01 |
| SC-03 Ответ из базы | q-01 → kb-03, kb-04 |
| SC-04 Эксперт → текст → страница | q-02, expert-answers (u-orlov), kb-20 после сценария |
| SC-05 Отказ от отправки | q-02, вариант `refuse_variant` |
| SC-06 Срочный вопрос | q-11, expert-answers, notifications `urgent_question` |
| SC-07 Эксперт не определён → бадди | q-04, expert-answers (u-belova) |
| SC-08 «Не ко мне» | q-05, kb-07, expert-answers (not_mine → u-orlov) |
| SC-09 Бадди переадресует | q-12, expert-answers (redirect) |
| SC-10 Встреча с записью | q-03, calendar expected_slots, MTG-301, transcripts/MTG-301.md |
| SC-11 Встреча без записи | q-10, MTG-302, expert-answers (decision: no) |
| SC-12 Эффект базы | q-06, kb-20, tracker scenario_variants |
| SC-13 Скрытый текст вопроса | team (u-egorova.settings) |
| SC-14 Отгул | q-09, kb-02, absence-service accepted |
| SC-15 Ошибка сервиса | absence-service unavailable |
| SC-16 Нет прав | q-07, kb-R1, access-service forbidden |
| SC-17 Граница ИИ | q-08 |
| SC-18 Просроченный шаг | tracker s-04 Overdue, calendar ev-05 |
| SC-19 Простой | tracker scenario_variants |
| SC-20 Черновик плана | tracker plan-egorova (draft) |
| SC-21 Дашборд руководителя | tracker, expert-answers, события сценариев |
| SC-22 Поддержка базы и дубли | kb-pages maintenance_seed, expert-answers (assign_open_question) |
| SC-23 SLA истёк | expert-answers sla_variant |
| SC-24 Telegram | notifications (answer_ready, привязка LINK-7Q2M) |
| SC-25 Маскот засыпает | notifications mascot_timers |
| SC-26 LLM недоступна | llm-stub |
| SC-27 Доступ по шагу плана | q-14, access-service, kb-11, tracker s-04 |
| SC-28 Отказ на ПДн | q-13 |
| SC-29 Квитанция сохраняется | absence-service persistence_check |
