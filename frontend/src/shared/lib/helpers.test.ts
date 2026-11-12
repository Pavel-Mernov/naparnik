import { describe, expect, it } from "vitest";
import type { PlanStep, Question, RequestItem } from "@/shared/api";
import { answerDue, buildNotes, dueText, initials, plural, relDay, requestTitle, statusOf } from "./helpers";
import { viewLabel, viewsFor, primaryRole } from "./state";

const step = (p: Partial<PlanStep>): PlanStep => ({ id: "s-1", title: "Шаг", stage: "st", owner: "intern", due_date: "2026-11-11", status: "Today", source: "", ...p });

describe("даты и склонения по-человечески", () => {
  it("склоняет", () => {
    expect([1, 2, 5, 11, 21, 22, 25].map((n) => plural(n, "шаг", "шага", "шагов"))).toEqual(["шаг", "шага", "шагов", "шагов", "шаг", "шага", "шагов"]);
  });
  it("относительные дни", () => {
    expect(relDay("2026-11-11", "2026-11-11")).toBe("сегодня");
    expect(relDay("2026-11-12", "2026-11-11")).toBe("завтра");
    expect(relDay("2026-11-10", "2026-11-11")).toBe("вчера");
    expect(relDay("2026-11-19", "2026-11-11")).toContain("19 ноября");
  });
  it("срок шага", () => {
    expect(dueText(step({ due_date: "2026-11-10", status: "Overdue" }), "2026-11-11")).toBe("срок был вчера");
    expect(dueText(step({}), "2026-11-11")).toBe("срок сегодня");
    expect(dueText(step({ status: "Done" }), "2026-11-11")).toBe("сделано");
  });
});

describe("статусы и подписи", () => {
  it("каждый статус — слово и тон, неизвестный не ломается", () => {
    expect(statusOf("granted")).toEqual({ label: "Доступ выдан", tone: "ok" });
    expect(statusOf("weird").label).toBe("weird");
  });
  it("заявки по-русски", () => {
    const base = { id: "A", employee_id: "u", approver_id: "l", status: "pending", created_at: "" };
    expect(requestTitle({ ...base, kind: "absence", type: "day_off", date: "2026-11-16" })).toBe("Отгул · 16 ноября");
    expect(requestTitle({ ...base, kind: "access", system: "sys-redash", title: "Redash (дашборды)" })).toBe("Доступ: Redash (дашборды)");
  });
  it("инициалы", () => expect(initials("Даниил Соколов")).toBe("ДС"));
});

describe("роли видят свои разделы", () => {
  it("руководитель без плана, эксперт без отдельных вопросов, стажёр — всё", () => {
    expect(viewsFor("intern")).toEqual(["day", "plan", "questions", "knowledge", "requests"]);
    expect(viewsFor("lead")).not.toContain("plan");
    expect(viewsFor("expert")).toEqual(["day", "knowledge"]);
    expect(viewLabel("lead", "requests")).toBe("Согласование");
    expect(primaryRole(["buddy", "expert"])).toBe("buddy");
  });
  it("выбирает приоритет роли и подписи каждого раздела", () => {
    expect(primaryRole(["lead", "buddy", "intern"])).toBe("lead");
    expect(primaryRole(["intern"])).toBe("intern");
    expect(primaryRole(["expert"])).toBe("expert");
    expect(viewLabel("intern", "day")).toBe("Мой день");
    expect(viewLabel("buddy", "plan")).toBe("Планы");
    expect(viewLabel("intern", "requests")).toBe("Заявки");
    expect(viewLabel("intern", "questions")).toBe("Вопросы");
    expect(viewLabel("buddy", "questions")).toBe("Вопросы стажёров");
    expect(viewLabel("expert", "questions")).toBe("Вопросы мне");
    expect(viewLabel("expert", "knowledge")).toBe("База знаний");
    expect(viewsFor("buddy")).toContain("plan");
  });
});

describe("уведомления — из данных, срочное первым", () => {
  const q = (p: Partial<Question>): Question => ({ id: "Q", asked_by: "u-s", addressee_id: "u-o", addressee: "Михаил", urgent: false, status: "Sent", ...p });
  const r = (p: Partial<RequestItem>): RequestItem => ({ id: "ABS-1", kind: "absence", type: "day_off", date: "2026-11-16", employee_id: "u-s", approver_id: "u-l", status: "pending", created_at: "", ...p });
  it("стажёр", () => {
    const notes = buildNotes({ meId: "u-s", today: "2026-11-11", steps: [step({ status: "Overdue", due_date: "2026-11-10", title: "Redash" })],
      questions: [q({ status: "Answered", title: "Конверсия" })], requests: [r({ status: "approved" })], drafts: [], events: [{ title: "1:1", start: "15:00" }] });
    expect(notes[0]).toMatchObject({ text: "Просрочен шаг: Redash", urgent: true });
    expect(notes.map((n) => n.text)).toContain("Отгул · 16 ноября: согласовано");
    expect(notes.at(-1)?.text).toBe("15:00 · 1:1");
  });
  it("эксперт и руководитель", () => {
    expect(buildNotes({ meId: "u-o", today: "2026-11-11", steps: [], questions: [q({})], requests: [], drafts: [{ id: "kb-20", title: "Конверсия" }], events: [] })
      .filter((n) => n.urgent)).toHaveLength(2);
    expect(buildNotes({ meId: "u-l", today: "2026-11-11", steps: [], questions: [], requests: [r({})], drafts: [], events: [] })[0].go).toBe("requests");
  });
});

describe("срок ответа на вопрос (SC-23)", () => {
  it("показывает срок только у ждущего ответа и отмечает просрочку", () => {
    expect(answerDue({ status: "Answered", due_at: "2026-11-12T14:00:00+03:00" })).toBe("");
    expect(answerDue({ status: "Sent" })).toBe("");
    expect(answerDue({ status: "Sent", due_at: "2026-11-12T14:00:00+03:00" })).toMatch(/^ответить до .*12/);
    expect(answerDue({ status: "Sent", due_at: "2026-11-12T14:00:00+03:00", overdue: true })).toMatch(/^срок ответа истёк/);
  });
  it("просроченный входящий вопрос выделен в уведомлениях", () => {
    const q = { id: "q1", asked_by: "u2", addressee_id: "u1", addressee: "Я", title: "Где дашборд?", urgent: false, status: "Sent", overdue: true } as Question;
    const notes = buildNotes({ meId: "u1", today: "2026-11-12", steps: [], questions: [q], requests: [], drafts: [], events: [] });
    expect(notes[0]).toMatchObject({ text: "Тебе вопрос — срок ответа истёк", urgent: true });
  });
});
