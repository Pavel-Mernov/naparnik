// Функции, которые были у старого фронта и вернулись в новый: встречи, «Не ко мне» и переадресация,
// правка черновика вопроса, оценка ответа, практика, Telegram, польза и поддержка базы у руководителя.
// Данные — настоящие ответы backend (src/test/fixtures.json), состояния подменяются в mockApi.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import fixtures from "@/test/fixtures.json";
import { mockApi, type Override } from "@/test/mockApi";
import App from "./App";

const fx = fixtures as Record<string, Record<string, unknown>>;
const start = (login: string, override?: Override) => {
  localStorage.setItem("naparnik.login", login);
  localStorage.setItem("naparnik.theme", "light");
  const api = mockApi(override);
  render(<App />);
  return api;
};
const openChat = async () => userEvent.click(await screen.findByRole("button", { name: "Открыть чат с Напарником" }));

beforeEach(() => localStorage.clear());
afterEach(() => vi.unstubAllGlobals());

// Вопрос эксперту в нужном состоянии — на основе настоящего вопроса Даниила.
const expertQuestion = (patch: Record<string, unknown>) => {
  const q = (fx["intern.sokolov"]["/api/questions"] as { questions: Array<Record<string, unknown>> }).questions[0];
  return { questions: [{ ...q, id: "Q-500", addressee_id: "u-smirnova", addressee: "Ольга Смирнова", status: "Sent", answer: undefined, page_id: undefined, ...patch }] };
};

describe("вопросы: встречи, «не ко мне», переадресация", () => {
  it("эксперт назначает встречу на свободный слот", async () => {
    const api = start("expert.smirnova", (c) => {
      if (c.path === "/api/questions" && c.method === "GET") return { body: expertQuestion({}) };
      if (c.path === "/api/questions/Q-500/meeting-slots") return { body: { slots: ["2026-11-12T11:00:00+03:00", "2026-11-12T14:00:00+03:00"] } };
      return undefined;
    });
    await userEvent.click(await screen.findByRole("button", { name: "Проще голосом — назначить встречу" }));
    const dialog = await screen.findByRole("dialog", { name: "Обсудить голосом" });
    await userEvent.click((await within(dialog).findAllByRole("button", { name: /12 ноября/ }))[0]);
    await waitFor(() => expect(api.writesTo("/api/questions/Q-500/meeting")).toHaveLength(1));
    expect(api.writesTo("/api/questions/Q-500/meeting")[0].body).toEqual({ slot: "2026-11-12T11:00:00+03:00" });
  });

  it("без общих слотов — совет ответить текстом", async () => {
    start("expert.smirnova", (c) => {
      if (c.path === "/api/questions" && c.method === "GET") return { body: expertQuestion({}) };
      if (c.path === "/api/questions/Q-500/meeting-slots") return { body: { slots: [] } };
      return undefined;
    });
    await userEvent.click(await screen.findByRole("button", { name: "Проще голосом — назначить встречу" }));
    expect(await screen.findByText(/лучше ответить текстом/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Отмена" }));
  });

  it("после встречи: «записана — собрать ответ» и «записи не было»", async () => {
    const meeting = { id: "MTG-303", question_id: "Q-500", organizer_id: "u-smirnova", participants: ["u-smirnova", "u-sokolov"], start: "2026-11-12T11:00:00+03:00", end: "2026-11-12T11:30:00+03:00", status: "scheduled" };
    const api = start("expert.smirnova", (c) => {
      if (c.path === "/api/questions" && c.method === "GET") return { body: expertQuestion({ status: "MeetingScheduled" }) };
      if (c.path === "/api/meetings") return { body: { meetings: [meeting] } };
      return undefined;
    });
    expect(await screen.findByText(/📅 Встреча:/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Встреча записана — собрать ответ" }));
    await waitFor(() => expect(api.writesTo("/api/meetings/MTG-303/finish")[0]?.body).toEqual({ recorded: true }));
    await userEvent.click(await screen.findByRole("button", { name: "Записи не было" }));
    await waitFor(() => expect(api.writesTo("/api/meetings/MTG-303/finish")[1]?.body).toEqual({ recorded: false }));
  });

  it("записи нет — ответить текстом или оставить вопрос открытым", async () => {
    const api = start("expert.smirnova", (c) => (c.path === "/api/questions" && c.method === "GET" ? { body: expertQuestion({ status: "NoRecording" }) } : undefined));
    expect(await screen.findByText(/Записи встречи нет/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Нет, оставить открытым" }));
    await waitFor(() => expect(api.writesTo("/api/questions/Q-500/leave-open")).toHaveLength(1));
  });

  it("«Не ко мне» — пусть Напарник подберёт следующего", async () => {
    const api = start("expert.smirnova", (c) => (c.path === "/api/questions" && c.method === "GET" ? { body: expertQuestion({}) } : undefined));
    await userEvent.click(await screen.findByRole("button", { name: "Не ко мне" }));
    const dialog = await screen.findByRole("dialog", { name: "Не ко мне" });
    await userEvent.click(within(dialog).getByRole("combobox"));
    await userEvent.click(await screen.findByRole("option", { name: /пусть Напарник подберёт/ }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Передать" }));
    await waitFor(() => expect(api.writesTo("/api/questions/Q-500/not-mine")).toHaveLength(1));
  });

  it("бадди переадресует вопрос стажёра и видит маршрут", async () => {
    const q = (fx["buddy.belova"]["/api/questions"] as { questions: Array<Record<string, unknown>> }).questions;
    const routed = { questions: [{ ...q[0], status: "Sent", answer: undefined, route: [{ at: "2026-11-11T10:00:00+03:00", from: "u-orlov", to: "u-smirnova", by: "u-orlov", kind: "not-mine" }] }] };
    const api = start("buddy.belova", (c) => (c.path === "/api/questions" && c.method === "GET" ? { body: routed } : undefined));
    await userEvent.click(await screen.findByRole("button", { name: "Вопросы стажёров" }));
    expect(await screen.findByText(/Маршрут: Михаил → Ольга/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Переадресовать" }));
    const dialog = await screen.findByRole("dialog", { name: "Переадресовать вопрос" });
    await userEvent.click(within(dialog).getByRole("combobox"));
    await userEvent.click((await screen.findAllByRole("option"))[0]);
    await userEvent.click(within(dialog).getByRole("button", { name: "Переадресовать" }));
    await waitFor(() => expect(api.calls.some((c) => c.method === "POST" && /\/redirect$/.test(c.path))).toBe(true));
  });

  it("стажёр видит запись встречи по своему вопросу", async () => {
    const meeting = { id: "MTG-303", question_id: "Q-101", organizer_id: "u-smirnova", participants: ["u-smirnova", "u-sokolov"], start: "2026-11-12T11:00:00+03:00", end: "2026-11-12T11:30:00+03:00", status: "recorded", recording_url: "https://vks.mock.local/rec/MTG-303" };
    start("intern.sokolov", (c) => (c.path === "/api/meetings" ? { body: { meetings: [meeting] } } : undefined));
    await userEvent.click(await screen.findByRole("button", { name: "Вопросы" }));
    expect(await screen.findByRole("link", { name: "Открыть запись встречи" })).toHaveAttribute("href", "https://vks.mock.local/rec/MTG-303");
  });
});

describe("чат: правка черновика вопроса и оценка ответа", () => {
  const question = {
    text: "Подготовил вопрос — проверь.", degraded: false, retry: false, sources: [],
    drafts: [{ id: "D-Q", tool: "send_question", args: { addressee_id: "u-belova", reason: "бадди", urgent: false, text: "Где шаблон презентации?" }, version: 1 }],
  };
  it("стажёр правит текст и срочность, сохраняет и подтверждает", async () => {
    const api = start("intern.sokolov", (c) => (c.path === "/api/chat/messages" ? { body: question } : undefined));
    await screen.findByRole("heading", { name: /Даниил, на сегодня/ });
    await openChat();
    await userEvent.type(screen.getByPlaceholderText("Спроси о команде или задачах"), "Где шаблон?{Enter}");
    await userEvent.click(await screen.findByRole("button", { name: "Изменить вопрос" }));
    const field = screen.getByRole("textbox", { name: "Текст вопроса" });
    await userEvent.clear(field);
    await userEvent.type(field, "Где шаблон презентации для экватора?");
    await userEvent.click(screen.getByRole("checkbox", { name: /Срочно/ }));
    await userEvent.click(screen.getByRole("button", { name: "Сохранить изменения" }));
    await waitFor(() => expect(api.calls.find((c) => c.method === "PATCH" && c.path === "/api/drafts/D-Q")?.body)
      .toEqual({ version: 1, text: "Где шаблон презентации для экватора?", urgent: true }));
    expect(await screen.findByText("срочно — сразу")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Подтвердить" }));
    await waitFor(() => expect(api.writesTo("/api/drafts/D-Q/confirm")[0]?.body).toEqual({ version: 2 }));
  });

  it("правку можно не сохранять; черновик, которого нет на сервере, можно обновить", async () => {
    let missing = true;
    start("intern.sokolov", (c) => {
      if (c.path === "/api/chat/messages") return { body: { ...question, drafts: [{ ...question.drafts[0], id: "D-GONE" }] } };
      if (c.path === "/api/drafts/D-GONE" && c.method === "GET" && missing) { missing = false; return { status: 404, body: { error: { message: "Черновик не найден." } } }; }
      return undefined;
    });
    await screen.findByRole("heading", { name: /Даниил, на сегодня/ });
    await openChat();
    await userEvent.type(screen.getByPlaceholderText("Спроси о команде или задачах"), "Вопрос{Enter}");
    expect(await screen.findByText("Черновик не найден.")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Обновить черновик" }));
    await userEvent.click(await screen.findByRole("button", { name: "Изменить вопрос" }));
    await userEvent.click(screen.getByRole("button", { name: "Не сохранять" }));
    expect(screen.getByRole("button", { name: "Изменить вопрос" })).toBeInTheDocument();
  });

  it("оценка ответа из базы уходит на сервер, «не помогло» зовёт человека", async () => {
    const api = start("intern.sokolov", (c) => (c.path === "/api/chat/messages" && (c.body as { text: string }).text === "Что такое витрина?"
      ? { body: { text: "Витрина — таблица.", degraded: false, retry: false, drafts: [], sources: [{ page_id: "kb-03", title: "Витрины DWH", author: "Михаил Орлов", updated_at: "2026-10-28" }] } } : undefined));
    await screen.findByRole("heading", { name: /Даниил, на сегодня/ });
    await openChat();
    await userEvent.type(screen.getByPlaceholderText("Спроси о команде или задачах"), "Что такое витрина?{Enter}");
    await userEvent.click((await screen.findAllByRole("button", { name: "Не помогло" })).at(-1)!);
    await waitFor(() => expect(api.writesTo("/api/feedback").at(-1)?.body).toEqual({ page_id: "kb-03", value: "not_helped" }));
    await waitFor(() => expect(api.writesTo("/api/chat/messages").at(-1)?.body).toEqual({ text: "Не помогло, нужен человек: Что такое витрина?" }));
  });
});

describe("Telegram и уведомления", () => {
  it("подключение: код, ссылка на бота, отключение", async () => {
    let connected = false;
    const status = (c: { path: string }) => (c.path === "/api/notifications"
      ? { body: { ...(fx["intern.sokolov"]["/api/notifications"] as object), telegram: { bot: "naparnik_bot", connected, mode: "bot" } } } : undefined);
    const api = start("intern.sokolov", (c) => {
      if (c.path === "/api/telegram/link" && c.method === "POST") return { body: { code: "123456", link: "https://t.me/naparnik_bot?start=123456", bot: "naparnik_bot", mode: "bot" } };
      return status(c);
    });
    await screen.findByRole("heading", { name: /Даниил, на сегодня/ });
    await userEvent.click(screen.getByRole("button", { name: "Профиль и тестовые учётки" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: /Уведомления в Telegram/ }));
    const dialog = await screen.findByRole("dialog", { name: "Уведомления в Telegram" });
    await userEvent.click(within(dialog).getByRole("button", { name: "Получить код" }));
    expect(await within(dialog).findByText("123456")).toBeInTheDocument();
    expect(within(dialog).getByRole("link", { name: "Открыть @naparnik_bot" })).toHaveAttribute("href", "https://t.me/naparnik_bot?start=123456");
    connected = true;
    expect(await within(dialog).findByText("✓ Telegram подключён", {}, { timeout: 4000 })).toBeInTheDocument();
    await userEvent.click(within(dialog).getByRole("button", { name: "Отключить" }));
    expect(api.calls.some((c) => c.method === "DELETE" && c.path === "/api/telegram/link")).toBe(true);
    await userEvent.click(within(dialog).getByRole("button", { name: "Готово" }));
  });

  it("бот в режиме журнала и сбой получения кода", async () => {
    start("intern.sokolov", (c) => (c.path === "/api/telegram/link" ? { status: 503, body: { error: { message: "Telegram недоступен" } } } : undefined));
    await screen.findByRole("heading", { name: /Даниил, на сегодня/ });
    await userEvent.click(screen.getByRole("button", { name: "Профиль и тестовые учётки" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: /Уведомления в Telegram/ }));
    await userEvent.click(await screen.findByRole("button", { name: "Получить код" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Telegram недоступен");
  });

  it("серверные напоминания попадают в колокольчик", async () => {
    const notes = { notifications: [{ id: "n-1", type: "morning_plan", text: "Доброе утро! На сегодня 3 шага\nПервый — доступ к Redash", view: "plan", read: false },
      { id: "n-2", type: "page_stale", text: "Страница устарела", view: "knowledge", read: false }], telegram: { connected: false, mode: "log", bot: "" } };
    start("intern.sokolov", (c) => (c.path === "/api/notifications" ? { body: notes } : undefined));
    await screen.findByRole("heading", { name: /Даниил, на сегодня/ });
    await userEvent.click(await screen.findByRole("button", { name: /^Уведомления:/ }));
    await userEvent.click(await screen.findByText("Доброе утро! На сегодня 3 шага"));
  });
});

describe("руководитель: польза Напарника и поддержка базы", () => {
  it("назначает открытый вопрос, объединяет дубли, напоминает о черновике", async () => {
    const api = start("lead.kovalev");
    expect(await screen.findByText("Польза Напарника")).toBeInTheDocument();
    expect(await screen.findByText("Поддержка базы знаний")).toBeInTheDocument();
    await userEvent.click(await screen.findByRole("button", { name: "Назначить" }));
    const dialog = await screen.findByRole("dialog", { name: "Кто ответит?" });
    await userEvent.click(within(dialog).getByRole("combobox"));
    await userEvent.click((await screen.findAllByRole("option"))[1]);
    await userEvent.click(within(dialog).getAllByRole("button").at(-1)!);
    await waitFor(() => expect(api.calls.some((c) => c.method === "POST" && /\/assign$/.test(c.path))).toBe(true));
    await userEvent.click((await screen.findAllByRole("button", { name: "Объединить" }))[0]);
    await waitFor(() => expect(api.calls.some((c) => c.method === "POST" && c.path.startsWith("/api/kb/duplicates/"))).toBe(true));
    await userEvent.click((await screen.findAllByRole("button", { name: "Не дубль" }))[0]);
    await userEvent.click((await screen.findAllByRole("button", { name: "Напомнить" }))[0]);
    await waitFor(() => expect(api.calls.some((c) => c.method === "POST" && /\/remind$/.test(c.path))).toBe(true));
  });
});

// Практика перед задачей (SC-30…32): этапы стажёра и одобрение бадди. Состояние — из настоящего ответа backend.
const practice = (patch: Record<string, unknown>) => {
  const p = (fx["intern.sokolov"]["/api/practices"] as { data: Array<Record<string, unknown>> }).data[0];
  return { ok: true, data: [{ ...p, ...patch }] };
};
const task = { text: "Посчитай конверсию карточки в контакт по **ads_mart** за неделю.", dataset: "sandbox.ads_mart_sample", environment: "песочница", criteria: ["Деление без потери дробной части", "Таблица названа по правилам"] };

describe("практика перед задачей", () => {
  it("стажёр начинает практику и проходит термины", async () => {
    const api = start("intern.sokolov");
    await userEvent.click(await screen.findByRole("button", { name: "Начать практику" }));
    await waitFor(() => expect(api.writesTo("/api/practices/u-sokolov/pr-01/advance")[0]?.body).toEqual({ action: "start", expected_stage: "offered" }));
    await userEvent.click(screen.getByRole("button", { name: "Сразу к реальной задаче" }));
    await waitFor(() => expect(api.writesTo("/api/practices/u-sokolov/pr-01/advance")).toHaveLength(2));
  });

  it("этапы терминов: общие, команды с правилами, переход дальше", async () => {
    let stage = "general_terms";
    const api = start("intern.sokolov", (c) => (c.path === "/api/practices" && c.method === "GET" ? { body: practice({ stage }) } : undefined));
    expect(await screen.findByText(/Конверсия · общее определение/)).toBeInTheDocument();
    stage = "team_terms";
    await userEvent.click(screen.getByRole("button", { name: "Дальше" }));
    expect(await screen.findByText(/ads_mart · у нас так говорят/)).toBeInTheDocument();
    expect(screen.getByText("• Код-стайл SQL")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Пропустить этот этап" }));
    await waitFor(() => expect(api.writesTo("/api/practices/u-sokolov/pr-01/advance").at(-1)?.body).toMatchObject({ action: "skip_stage" }));
    await userEvent.click(screen.getAllByRole("button", { name: /Код-стайл SQL|Частые ошибки/ })[0]);
    expect(await screen.findByRole("button", { name: "Закрыть источник" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Закрыть источник" }));
  });

  it("учебное задание: попытка с ошибкой, разбор, вопрос бадди, затем успешная попытка и чек-лист", async () => {
    let state: Record<string, unknown> = { stage: "training", task, attempts: [], attempts_count: 0, help_available: false };
    const fail = { number: 1, answer: "select 1/2", review: { passed: false, score: 50, checks: [{ criterion: 1, passed: false, feedback: "Деление целых — получится 0", sources: ["kb-33"] }, { criterion: 2, passed: true, feedback: "Название по правилам" }] } };
    const api = start("intern.sokolov", (c) => {
      if (c.path === "/api/practices" && c.method === "GET") return { body: practice(state) };
      if (c.path.endsWith("/help-draft")) return { body: { ok: true, data: { id: "D-HELP", tool: "send_question", args: { addressee_id: "u-belova", text: "Не получается деление" }, version: 1, status: "pending" } } };
      return undefined;
    });
    expect(await screen.findByText(/Посчитай конверсию карточки/)).toBeInTheDocument();
    await userEvent.type(screen.getByRole("textbox", { name: "Твоя попытка" }), "select 1/2");
    state = { ...state, attempts: [fail], attempts_count: 1, help_available: true };
    await userEvent.click(screen.getByRole("button", { name: "Проверить попытку" }));
    expect(await screen.findByText("Есть что поправить · 50%")).toBeInTheDocument();
    expect(api.calls.find((c) => c.path.endsWith("/attempts"))?.body).toEqual({ answer: "select 1/2", task_version: 1 });
    await userEvent.click(screen.getByRole("button", { name: "Спросить бадди" }));
    expect(await screen.findByText("Черновик вопроса бадди")).toBeInTheDocument();

    state = { ...state, attempts: [fail, { ...fail, number: 2, review: { passed: true, score: 100, checks: [] } }], attempts_count: 2 };
    await userEvent.click(screen.getByRole("button", { name: "Пропустить учебное задание" }));
    expect(await screen.findByRole("button", { name: "К реальной задаче" })).toBeInTheDocument();
    state = { ...state, stage: "real_task", checklist: ["Собери витрину", "Сделай дашборд"] };
    await userEvent.click(screen.getByRole("button", { name: "К реальной задаче" }));
    expect(await screen.findByText("Собери витрину")).toBeInTheDocument();
  });

  it("задание не одобрено вовремя — стажёр продолжает без него", async () => {
    start("intern.sokolov", (c) => (c.path === "/api/practices" && c.method === "GET" ? { body: practice({ stage: "training", task: undefined, approval_expired: true }) } : undefined));
    expect(await screen.findByText(/Бадди не ответил за рабочий день/)).toBeInTheDocument();
  });

  it("бадди готовит и одобряет задание, видит лучшую попытку и темы ошибок", async () => {
    let state: Record<string, unknown> = { task_status: "Pending", task: undefined };
    const api = start("buddy.belova", (c) => (c.path === "/api/practices" && c.method === "GET" ? { body: practice(state) } : undefined));
    const prepare = await screen.findByRole("button", { name: "Подготовить задание" });
    state = { task_status: "Draft", task };
    await userEvent.click(prepare);
    const draft = await screen.findByRole("textbox", { name: "Черновик учебного задания" });
    await userEvent.type(draft, " Только активные объявления.");
    state = { task_status: "Approved", task, attempts_count: 2, error_topics: ["деление целых чисел"],
      best_attempt: { number: 2, answer: "select 1.0*a/b", review: { passed: true, score: 100, checks: [] } } };
    await userEvent.click(screen.getByRole("button", { name: "Одобрить задание" }));
    await waitFor(() => expect(api.calls.find((c) => c.path.endsWith("/approve"))?.body).toMatchObject({ task_version: 1 }));
    expect(await screen.findByText("Задание одобрено")).toBeInTheDocument();
    expect(screen.getByText("Лучшая попытка №2 · 100%")).toBeInTheDocument();
    expect(screen.getByText("деление целых чисел")).toBeInTheDocument();
  });

  it("сбой подготовки практики — понятная ошибка и «Повторить»", async () => {
    const api = start("intern.sokolov", (c) => (c.path === "/api/practices/sync" ? { status: 503, body: { error: { message: "Практика недоступна" } } } : undefined));
    expect(await screen.findByText("Практика недоступна")).toBeInTheDocument();
    await userEvent.click(screen.getAllByRole("button", { name: "Повторить" })[0]);
    await waitFor(() => expect(api.writesTo("/api/practices/sync").length).toBeGreaterThan(1));
  });
});
