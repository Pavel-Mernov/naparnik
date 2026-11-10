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
};

export type PlanData = {
  intern: string;
  stages: Array<{ id: string; title: string; order: number }>;
  steps: PlanStep[];
  progress_pct: number;
  next_step?: PlanStep;
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
  text?: string;
  text_hidden?: boolean;
  urgent: boolean;
  status: string;
};

export type RequestItem = {
  id: string;
  kind: string;
  type?: string;
  date?: string;
  system?: string;
  employee_id: string;
  approver_id: string;
  status: string;
  created_at: string;
};

export type KbPage = {
  id: string;
  folder: string;
  type: string;
  title: string;
  excerpt: string;
  author: string;
  updated_at: string;
  test_instruction: boolean;
};

export type KbFolder = { id: string; title: string; space: string };

export type ChatReply = {
  text: string;
  drafts: Array<{ id: string; tool: string; args: Record<string, unknown> }>;
  sources: Array<{ page_id: string; title: string; author: string; updated_at: string; label?: string }>;
  degraded: boolean;
  retry: boolean;
};

type ApiEnvelope<T> = { ok: boolean; data?: T; error?: { code: string; message: string } };

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

export const api = async <T>(login: string, path: string, init: RequestInit = {}): Promise<T> => {
  const response = await fetch(path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      "X-User": login,
      ...init.headers,
    },
  });
  const payload = (await response.json().catch(() => ({}))) as ApiEnvelope<T> | T;
  if (!response.ok) {
    const envelope = payload as ApiEnvelope<T>;
    throw new ApiError(response.status, envelope.error?.message ?? "Не удалось выполнить запрос");
  }
  return payload as T;
};

export const unwrap = <T>(value: ApiEnvelope<T>): T => {
  if (!value.ok || value.data === undefined) throw new Error(value.error?.message ?? "Нет данных");
  return value.data;
};
