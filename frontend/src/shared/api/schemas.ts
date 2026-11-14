// Проверка ответов сервера (Zod): если backend вернул данные не той формы, экран не падает на
// «undefined is not a function», а показывает понятную ошибку. Схемы проверяют поля, на которые
// опирается интерфейс; остальные поля проходят как есть (looseObject), поэтому новые поля backend
// не ломают веб.
import { z } from "zod";

const str = z.string();
const opt = <T extends z.ZodType>(s: T) => s.optional().nullable();
// Go отдаёт пустой список как null — это не ошибка.
const list = <T extends z.ZodType>(s: T) => z.array(s).nullable();

const user = z.looseObject({ id: str, login: str, name: str, position: str, roles: list(str) });
const step = z.looseObject({ id: str, title: str, stage: str, owner: str, due_date: str, status: str });
const reply = z.looseObject({
  text: str,
  drafts: list(z.looseObject({ id: str, tool: str, args: z.record(str, z.unknown()) })),
  sources: list(z.looseObject({ page_id: str, title: str })),
});

export const schemas = {
  me: user.extend({ week: z.number(), today: str, department: str, interns: list(z.looseObject({ id: str, name: str })) }),
  users: z.looseObject({ users: list(user) }),
  plan: z.looseObject({ ok: z.boolean(), data: z.looseObject({ steps: list(step), stages: list(z.looseObject({ id: str, title: str })), progress_pct: z.number() }) }),
  today: z.looseObject({ ok: z.boolean(), data: z.looseObject({ week: z.number(), steps: list(step) }) }),
  questions: z.looseObject({ questions: list(z.looseObject({ id: str, asked_by: str, addressee_id: str, status: str, urgent: z.boolean() })) }),
  requests: z.looseObject({ requests: list(z.looseObject({ id: str, kind: str, status: str, employee_id: str, approver_id: str })) }),
  kbPages: z.looseObject({ folders: list(z.looseObject({ id: str, title: str })), pages: list(z.looseObject({ id: str, title: str, folder: str })) }),
  notifications: z.looseObject({ notifications: list(z.looseObject({ id: str, type: str, text: str, read: z.boolean() })) }),
  chatReply: reply,
  chatHistory: z.looseObject({ conversation: z.number(), turns: list(z.looseObject({ id: str, question: str, reply })) }),
  settings: z.looseObject({ settings: z.looseObject({ proactivity: str, reminders: list(str), working_hours: str, quiet_until: opt(str) }) }),
  overview: z.looseObject({ ok: z.boolean(), data: z.looseObject({ interns: list(z.looseObject({ id: str, name: str, progress_pct: z.number() })), totals: z.looseObject({ interns: z.number() }) }) }),
  kbOwner: z.looseObject({ owner_id: str, can_change: z.boolean(), is_owner: z.boolean() }),
  maintenance: z.looseObject({
    owner_id: str, next_check: str,
    open_questions: list(z.looseObject({ id: str, title: str })), drafts: list(z.looseObject({ id: str, author_id: str })),
    stale: list(z.looseObject({ page_id: str })), duplicates: list(z.looseObject({ id: str, titles: list(str) })),
  }),
};

// Какой схемой проверять ответ: по методу и пути (без query).
const routes: Array<[method: string, pattern: RegExp, schema: z.ZodType]> = [
  ["GET", /^\/api\/me$/, schemas.me],
  ["GET", /^\/api\/users$/, schemas.users],
  ["GET", /^\/api\/plans\/[^/]+$/, schemas.plan],
  ["GET", /^\/api\/today$/, schemas.today],
  ["GET", /^\/api\/questions$/, schemas.questions],
  ["GET", /^\/api\/requests$/, schemas.requests],
  ["GET", /^\/api\/kb\/pages$/, schemas.kbPages],
  ["GET", /^\/api\/notifications$/, schemas.notifications],
  ["POST", /^\/api\/chat\/messages$/, schemas.chatReply],
  ["GET", /^\/api\/chat\/history$/, schemas.chatHistory],
  ["GET", /^\/api\/settings$/, schemas.settings],
  ["PATCH", /^\/api\/settings$/, schemas.settings],
  ["GET", /^\/api\/dashboard\/team$/, schemas.overview],
  ["GET", /^\/api\/kb\/owner$/, schemas.kbOwner],
  ["PUT", /^\/api\/kb\/owner$/, schemas.kbOwner],
  ["GET", /^\/api\/kb\/maintenance$/, schemas.maintenance],
  ["POST", /^\/api\/kb\/check$/, schemas.maintenance],
];

export const schemaFor = (method: string, path: string): z.ZodType | undefined => {
  const pathname = path.split("?")[0];
  return routes.find(([m, re]) => m === method && re.test(pathname))?.[2];
};

/** Ответ не той формы: что именно не сошлось — в консоль для разработчика, человеку — понятный текст. */
export const checkShape = (method: string, path: string, payload: unknown): string | null => {
  const schema = schemaFor(method, path);
  if (!schema) return null;
  const result = schema.safeParse(payload);
  if (result.success) return null;
  console.error(`Ответ ${method} ${path} не той формы`, result.error.issues);
  return "Сервер прислал данные в неожиданном формате. Обнови страницу; если повторится — напиши в поддержку.";
};
