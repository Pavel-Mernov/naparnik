import { checkShape } from "./schemas";

export type User = {
  id: string;
  login: string;
  name: string;
  position: string;
  roles: string[];
};

export type Me = User & {
  week: number;
  today: string;
  department: string;
  interns: Array<{ id: string; name: string }>;
};

export type PlanStep = {
  id: string;
  title: string;
  stage: string;
  owner: string;
  due_date: string;
  status: "Planned" | "Today" | "Done" | "Overdue" | string;
  source: string;
  /** Готовое действие шага: «team_map» открывает карту команды. */
  action?: string;
};

/** Настройки человека (SCR-I8). */
export type Settings = {
  proactivity: "daily_plan" | "deadlines_and_meetings" | "off" | string;
  question_text_visible_to_buddy: boolean;
  reminders: string[];
  /** «Не беспокоить» до этого момента; нет — выключено. */
  quiet_until?: string;
  working_hours: string;
};

/** Человек на карте команды (SCR-I5, SC-02): зона ответственности, его страницы в базе, черновик сообщения. */
export type TeamCard = {
  id: string; name: string; initials: string; photo: { url: string; updated_at: string } | null; status: string; position: string;
  responsibility: string; working_hours: string; my_buddy?: boolean;
  /** Топ-руководитель: вопросы к нему — через бадди (BR-10a), черновика нет. */
  via_buddy?: boolean;
  pages: Array<{ id: string; title: string; updated_at: string }>;
  /** Черновик первого сообщения: человек копирует и отправляет сам (BR-62). */
  message: string;
};

export type PlanData = {
  intern: string;
  stages: Array<{ id: string; title: string; order: number }>;
  steps: PlanStep[];
  progress_pct: number;
  next_step?: PlanStep;
  /** BR-81: draft | approved | auto_activated; approval — пометка «не утверждён». */
  plan_status?: string;
  approval?: string;
};

export type TodayData = {
  week: number;
  steps: PlanStep[];
  events: Array<{ title: string; start: string }>;
};

export type Question = {
  id: string;
  asked_by: string;
  addressee_id: string;
  addressee: string;
  title?: string;
  text?: string;
  text_hidden?: boolean;
  answer?: string;
  page_id?: string;
  reason?: string;
  urgent: boolean;
  status: string;
  hint?: string; // BR-10a: подсказка только адресату-бадди
  route?: Array<{ at: string; from: string; to: string; by: string; kind: string }>;
  /** Срок ответа текущего адресата (BR-18, SC-23) — только у вопроса в статусе Sent. */
  due_at?: string;
  overdue?: boolean;
};

export type RequestItem = {
  id: string;
  kind: string;
  type?: string;
  date?: string;
  system?: string;
  title?: string;
  source?: string;
  comment?: string;
  step_id?: string;
  reason_code?: string;
  decision?: string;
  history?: Array<{ at: string; by: string; text: string }>;
  employee_id: string;
  approver_id: string;
  status: string;
  created_at: string;
};

export type InternOverview = {
  id: string; name: string; buddy: string; week: number; plan_status: string;
  stage: string; progress_pct: number; overdue_steps: number; open_questions: number;
};

export type TeamOverview = {
  department: string;
  interns: InternOverview[];
  requests: Array<{ id: string; kind: string; title: string; type?: string; date?: string;
    employee_id: string; employee_name: string; status: string }>;
  totals: { interns: number; overdue_steps: number; open_questions: number; pending_requests: number };
};

export type Services = {
  absence: { reason_codes: Array<{ code: string; text: string }>; types?: AbsenceType[]; approver?: string; self_service?: { steps: string[] } };
  access: { reason_codes: Array<{ code: string; text: string }>; systems?: SystemOption[] };
};

export type KbPage = {
  id: string;
  folder: string;
  type: string;
  title: string;
  excerpt: string;
  body?: string;
  author: string;
  updated_at: string;
  test_instruction: boolean;
};

export type Meeting = { id: string; question_id: string; organizer_id: string; participants: string[]; start: string; end: string; status: string; page_id?: string; recording_url?: string };
export type KbDraft = { id: string; title: string; body: string; folder: string; similar_page_id?: string; source_question?: string };
export type AbsenceType = { type: string; title: string };
export type SystemOption = { system: string; title: string; status: string; week_from: number; now: boolean };
export type Warning = { code: string; message: string };
export type KbFolder = { id: string; title: string; space: string };

export type ChatReply = {
  text: string;
  /** status и result приходят в истории: подтверждённый черновик показывается квитанцией. */
  drafts: Array<{ id: string; tool: string; args: Record<string, unknown>; version?: number; status?: "pending" | "confirmed" | "cancelled"; result?: { receipt?: Record<string, string> } }>;
  sources: Array<{ page_id: string; title: string; author: string; updated_at: string; label?: string }>;
  degraded: boolean;
  retry: boolean;
  /** Модель недоступна: частые действия роли, нажатие отправляет text как сообщение (BR-70). */
  actions?: Array<{ label: string; text: string }>;
};

/** Сохранённый вопрос Напарнику: окно чата после перезагрузки и «Спрашивал у Напарника». */
export type ChatTurn = { id: string; conversation: number; question: string; at: string; reply: ChatReply };
export type ChatHistory = { conversation: number; turns: ChatTurn[] };

type ApiEnvelope<T> = { ok: boolean; data?: T; error?: { code: string; message: string } };

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

const requestKeys = new Map<string, string>();

export const api = async <T>(login: string, path: string, init: RequestInit = {}): Promise<T> => {
  const headers = new Headers(init.headers);
  headers.set("Content-Type", "application/json");
  headers.set("X-User", login);
  const signature = init.method?.toUpperCase() === "POST" && path === "/api/requests"
    ? JSON.stringify([login, path, init.body]) : "";
  if (signature && !headers.has("Idempotency-Key")) {
    if (!requestKeys.has(signature)) requestKeys.set(signature, crypto.randomUUID());
    headers.set("Idempotency-Key", requestKeys.get(signature)!);
  }
  const response = await fetch(path, {
    ...init,
    headers,
  });
  if (signature && response.status < 500) requestKeys.delete(signature);
  const payload = (await response.json().catch(() => ({}))) as ApiEnvelope<T> | T;
  if (!response.ok) {
    const envelope = payload as ApiEnvelope<T>;
    throw new ApiError(response.status, envelope.error?.message ?? "Не удалось выполнить запрос");
  }
  const problem = checkShape((init.method ?? "GET").toUpperCase(), path, payload);
  if (problem) throw new ApiError(response.status, problem);
  return payload as T;
};

export const unwrap = <T>(value: ApiEnvelope<T>): T => {
  if (!value.ok || value.data === undefined) throw new Error(value.error?.message ?? "Нет данных");
  return value.data;
};

// ---------- Профили сотрудников (US-38, spec/05 «Профили сотрудников») ----------
export type Profile = {
  id: string;
  name: string;
  initials: string;
  photo: { url: string; updated_at: string } | null;
  status: string;
  position: string;
  department: string;
  phone: string;
  email: string;
  start_date?: string;
  week?: number;
  buddy?: string;
  lead?: string;
  can_edit: boolean;
};

export const MAX_PHOTO_BYTES = 512 * 1024;

export const getProfile = (login: string, id?: string, init: RequestInit = {}) =>
  api<{ ok: boolean; profile: Profile }>(login, id ? `/api/profiles/${encodeURIComponent(id)}` : "/api/profile", init).then((x) => x.profile);

export const updatePhone = (login: string, phone: string) =>
  api<{ ok: boolean; profile: Profile }>(login, "/api/profile", { method: "PATCH", body: JSON.stringify({ phone }) }).then((x) => x.profile);

export const deletePhoto = (login: string) =>
  api<{ ok: boolean; profile: Profile }>(login, "/api/profile/photo", { method: "DELETE" }).then((x) => x.profile);

/** Фото — тело запроса как есть (не JSON): тип бэкенд определяет по содержимому. */
export const uploadPhoto = async (login: string, file: Blob): Promise<Profile> => {
  const response = await fetch("/api/profile/photo", { method: "PUT", headers: { "X-User": login, "Content-Type": file.type || "application/octet-stream" }, body: file });
  const payload = (await response.json().catch(() => ({}))) as ApiEnvelope<never> & { profile?: Profile };
  if (!response.ok || !payload.profile) throw new ApiError(response.status, payload.error?.message ?? "Не удалось загрузить фото");
  return payload.profile;
};

/** Фото отдаётся только с X-User, поэтому <img> получает object URL; вызывающий освобождает его. */
export const fetchPhoto = async (login: string, url: string, signal?: AbortSignal): Promise<string> => {
  const response = await fetch(url, { headers: { "X-User": login }, signal });
  if (!response.ok) throw new ApiError(response.status, "Фото недоступно");
  return URL.createObjectURL(await response.blob());
};

// ---------- Черновик плана адаптации (SC-20, BR-81, spec/05 «Черновик плана») ----------
export type PlanDraftStep = { id: string; title: string; stage: string; owner: string; due_date: string; removed?: boolean };
export type PlanDraft = {
  intern_id: string;
  intern: string;
  status: "draft" | "approved" | "auto_activated" | string;
  created_at?: string;
  edited_by?: string;
  comment?: string;
  approved_by?: string;
  auto_activate_at?: string;
  can_edit: boolean;
  can_approve: boolean;
  stages: Array<{ id: string; title: string; order: number }>;
  steps: PlanDraftStep[];
};
export type PlanDraftEdit = { step_id?: string; due_date?: string; removed?: boolean };

export const getPlanDraft = (login: string, internId: string, init: RequestInit = {}) =>
  api<PlanDraft>(login, `/api/plans/${encodeURIComponent(internId)}/draft`, init);

export const editPlanDraft = (login: string, internId: string, steps: PlanDraftEdit[], comment: string) =>
  api<PlanDraft>(login, `/api/plans/${encodeURIComponent(internId)}/draft`, { method: "PATCH", body: JSON.stringify({ steps, comment }) });

export const approvePlan = (login: string, internId: string) =>
  api<PlanDraft>(login, `/api/plans/${encodeURIComponent(internId)}/approve`, { method: "POST", body: "{}" });

// ---------- Живые обновления (SSE) ----------
/**
 * Подписка на /api/stream: сервер присылает «changed», когда у человека что-то изменилось.
 * Читаем поток через fetch, а не EventSource: так уходит заголовок X-User, как у остальных запросов.
 * Поток оборвался — переподключаемся через retryMs; пока его нет, работает обычный опрос.
 */
export const subscribeChanges = (login: string, onChange: () => void, retryMs = 5000): (() => void) => {
  const controller = new AbortController();
  let timer: ReturnType<typeof setTimeout> | undefined;
  const connect = async () => {
    try {
      const response = await fetch("/api/stream", { headers: { "X-User": login, Accept: "text/event-stream" }, signal: controller.signal });
      const reader = response.ok ? response.body?.getReader() : undefined;
      if (!reader) throw new Error("нет потока");
      const decoder = new TextDecoder();
      let buffer = "";
      for (;;) {
        const { value, done } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        let end: number;
        while ((end = buffer.indexOf("\n\n")) >= 0) {
          const block = buffer.slice(0, end);
          buffer = buffer.slice(end + 2);
          if (block.split("\n").includes("event: changed")) onChange();
        }
      }
    } catch { /* сеть, 404 или отмена — ниже решаем, переподключаться ли */ }
    if (!controller.signal.aborted) timer = setTimeout(() => void connect(), retryMs);
  };
  void connect();
  return () => { controller.abort(); clearTimeout(timer); };
};
