// Имитация backend для интеграционных тестов приложения. GET-ответы — настоящие ответы backend в режиме
// заглушки модели (src/test/fixtures.json: отгул, вопрос эксперту, ответ эксперта, вопросы Напарнику).
// Записи (POST, PATCH, DELETE) по умолчанию успешны; тест может подменить любой ответ через override.
import { vi } from "vitest";
import fixtures from "./fixtures.json";

export type ApiCall = { method: string; path: string; user: string; body: unknown };
type Reply = { status?: number; body: unknown };
// Подмена ответа; Promise — ответ, который тест придерживает (запоздавший ответ старой учётки).
export type Override = (call: ApiCall) => Reply | undefined | Promise<Reply | undefined>;

const data = fixtures as Record<string, Record<string, unknown>>;

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

/** Ответ Напарника на сообщение в чате: черновик отгула с подтверждением. */
export const chatReply = (text = "Подготовил черновик — проверь и подтверди.") => ({
  text, degraded: false, retry: false,
  drafts: [{ id: "DRAFT-9", tool: "create_absence_request", args: { type: "day_off", date: "2026-11-13" }, version: 1, status: "pending" }],
  sources: [{ page_id: "kb-02", title: "Отгулы, отпуск и больничный: как оформить", author: "Андрей Ковалёв", updated_at: "2026-09-02", label: "тестовая инструкция, не регламент Авито" }],
});

type Draft = { id: string; tool: string; args: Record<string, unknown>; version?: number; status?: string; result?: unknown };
const RECEIPT = { id: "ABS-1099", status: "pending", approver: "Андрей Ковалёв" };

export const mockApi = (override?: Override) => {
  const calls: ApiCall[] = [];
  // Черновики, которые Напарник показал в чате: карточка перечитывает их с сервера, правка и отмена меняют состояние.
  const drafts = new Map<string, Draft>();
  const settings: Record<string, Record<string, unknown>> = {};
  // Владелец базы (BR-34): по умолчанию руководитель; «Поддержка базы» отдаётся только ему.
  let kbOwner = "u-kovalev";
  let lastCheck: string | undefined;
  const idOf = (login: string) => (data[login]?.["/api/me"] as { id?: string } | undefined)?.id ?? login;
  const maintenance = () => ({ ...(data["lead.kovalev"]["/api/kb/maintenance"] as Record<string, unknown>), owner_id: kbOwner, last_check: lastCheck });
  const ownerFor = (login: string) => ({ owner_id: kbOwner, default_id: "u-kovalev", is_owner: idOf(login) === kbOwner, can_change: idOf(login) === "u-kovalev" });
  for (const turns of Object.values(data).map((u) => (u["/api/chat/history"] as { turns?: Array<{ reply: { drafts: Draft[] } }> } | undefined)?.turns ?? []))
    for (const t of turns) for (const d of t.reply.drafts) drafts.set(d.id, d);
  const writes = (c: ApiCall, reply?: unknown): Reply => {
    const id = c.path.match(/^\/api\/drafts\/([^/]+)/)?.[1];
    if (c.path === "/api/chat/messages") return { body: reply ?? chatReply() };
    if (id && c.path.endsWith("/confirm")) { drafts.set(id, { ...drafts.get(id)!, status: "confirmed", result: { ok: true, receipt: RECEIPT } }); return { body: { ok: true, receipt: RECEIPT } }; }
    if (id && c.method === "DELETE") { drafts.set(id, { ...drafts.get(id)!, status: "cancelled" }); return { body: { ok: true } }; }
    if (id && c.method === "PATCH") {
      const d = drafts.get(id)!; const b = c.body as { text: string; urgent: boolean };
      drafts.set(id, { ...d, version: (d.version ?? 1) + 1, args: { ...d.args, text: b.text, urgent: b.urgent } });
      return { body: { ok: true } };
    }
    if (c.path === "/api/settings" && c.method === "PATCH") {
      const cur = (settings[c.user] ??= { ...(data[c.user]?.["/api/settings"] as { settings: Record<string, unknown> }).settings });
      const b = c.body as Record<string, unknown>;
      for (const k of ["proactivity", "question_text_visible_to_buddy", "reminders"]) if (k in b) cur[k] = b[k];
      if ("quiet_hours" in b) cur.quiet_until = b.quiet_hours ? "2026-11-11T18:00:00+03:00" : undefined;
      return { body: { ok: true, settings: cur } };
    }
    if (c.path === "/api/kb/owner" && c.method === "PUT") { kbOwner = (c.body as { user_id: string }).user_id; return { body: ownerFor(c.user) } }
    if (c.path === "/api/kb/check") { lastCheck = "2026-11-11"; return { body: maintenance() }; }
    if (c.path === "/api/requests") return { body: { ok: true, data: { id: "ABS-1100" } } };
    if (c.path === "/api/telegram/link") return { body: { code: "123456", link: "https://t.me/naparnik_bot?start=123456", expires_in: 600 } };
    return { body: { ok: true } };
  };
  const remember = (body: unknown) => { for (const d of (body as { drafts?: Draft[] })?.drafts ?? []) drafts.set(d.id, { status: "pending", ...d }); };
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = new URL(String(input), "http://localhost");
    const method = (init.method ?? "GET").toUpperCase();
    let body: unknown;
    try { body = typeof init.body === "string" ? JSON.parse(init.body) : init.body; } catch { body = init.body; }
    const call = { method, path: url.pathname + url.search, user: new Headers(init.headers).get("X-User") ?? "", body };
    calls.push(call);
    const custom = await override?.(call);
    if (custom && call.path === "/api/chat/messages" && (custom.status ?? 200) < 300) remember(custom.body);
    if (custom && !(call.path.endsWith("/confirm") && (custom.status ?? 200) < 300)) return json(custom.body, custom.status);
    if (method !== "GET") {
      const r = writes(call, custom?.body);
      if (call.path === "/api/chat/messages") remember(r.body);
      return json(r.body, r.status);
    }
    const draftId = url.pathname.match(/^\/api\/drafts\/([^/]+)$/)?.[1];
    if (draftId) return drafts.has(draftId) ? json({ ok: true, draft: drafts.get(draftId) }) : json({ error: { message: "Черновик не найден." } }, 404);
    if (url.pathname.startsWith("/api/services/absence/precheck")) return json({ ok: true, data: { warnings: [{ code: "LATE", message: "Меньше 2 рабочих дней до даты" }] } });
    if (url.pathname === "/api/kb/owner") return json(ownerFor(call.user));
    if (url.pathname === "/api/kb/maintenance") return idOf(call.user) === kbOwner ? json(maintenance()) : json({ error: { code: "forbidden", message: "Поддержка базы — у владельца базы." } }, 403);
    const found = data[call.user]?.[url.pathname];
    return found === undefined ? json({ error: { code: "not_found", message: "Нет данных" } }, 404) : json(found);
  });
  vi.stubGlobal("fetch", fetchMock);
  return { calls, fetchMock, writesTo: (path: string) => calls.filter((c) => c.method !== "GET" && c.path.startsWith(path)) };
};
