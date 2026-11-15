import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider } from "@mui/material";
import { describe, expect, it, vi } from "vitest";
import { makeTheme } from "@/shared/theme";
import type { PlanDraft } from "@/shared/api";
import { PlanDraftDialog } from "./PlanDraftDialog";

const base: PlanDraft = {
  intern_id: "u-sokolov", intern: "Даниил Соколов", status: "approved", version: 1, revision: 1, active_revision: 1, review_status: "",
  can_edit: false, can_approve: false, can_submit: false, can_return: false, can_start: true, can_cancel: false, changes: [], history: [],
  stages: [{ id: "st-w1", title: "Неделя 1", order: 1 }], steps: [
    { id: "s-01", title: "Готовый шаг", stage: "st-w1", owner: "intern", due_date: "2026-11-10", done: true },
    { id: "s-02", title: "Новый шаг", stage: "st-w1", owner: "intern", due_date: "2026-11-12", done: false },
  ],
};
const editing: PlanDraft = { ...base, version: 2, revision: 2, review_status: "editing", can_start: false, can_edit: true, can_submit: true, can_cancel: true };
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
const open = (login = "buddy.belova") => render(<ThemeProvider theme={makeTheme("light")}><PlanDraftDialog open login={login} internId="u-sokolov" onClose={vi.fn()} onDone={vi.fn()} /></ThemeProvider>);

describe("Полный цикл плана (SCR-L2)", () => {
  it("открывает редакцию активного плана, защищает выполненный шаг, сохраняет и отправляет версию", async () => {
    const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(json(base)).mockResolvedValueOnce(json(editing))
      .mockResolvedValueOnce(json({ ...editing, version: 3, comment: "Смена проекта" }))
      .mockResolvedValueOnce(json({ ...editing, version: 4, comment: "Смена проекта", review_status: "review", can_submit: false }));
    open();
    await userEvent.click(await screen.findByRole("button", { name: "Изменить действующий план" }));
    expect(await screen.findByLabelText("Срок: Готовый шаг")).toBeDisabled();
    expect(JSON.parse(String(fetch.mock.calls[1][1]?.body))).toEqual({ version: 1, comment: "" });
    await userEvent.type(screen.getByLabelText("Комментарий руководителю"), "Смена проекта");
    expect(screen.getByRole("button", { name: "Отправить на согласование" })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "Сохранить правки" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Отправить на согласование" })).toBeEnabled());
    await userEvent.click(screen.getByRole("button", { name: "Отправить на согласование" }));
    await screen.findByText("Действующая редакция: 1 · Редакция 2: На согласовании");
    expect(fetch.mock.calls[3][0]).toBe("/api/plans/u-sokolov/revisions/submit");
    expect(JSON.parse(String(fetch.mock.calls[3][1]?.body)).version).toBe(3);
  });

  it("руководитель возвращает редакцию только с замечанием и видит историю", async () => {
    const lead: PlanDraft = { ...base, version: 7, revision: 2, review_status: "review", can_start: false, can_return: true, can_approve: true };
    const returned: PlanDraft = { ...lead, version: 8, review_status: "returned", can_approve: false, can_return: false, return_comment: "Уточни сроки", history: [{ version: 8, revision: 2, action: "returned", by: "Андрей Ковалёв", at: "2026-11-15T10:00:00+03:00", comment: "Уточни сроки", steps: base.steps, changes: [{ step_id: "s-02", old_due_date: "2026-11-12", due_date: "2026-11-16", was_removed: false, removed: false }] }] };
    const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(json(lead)).mockResolvedValueOnce(json(returned));
    open("lead.kovalev");
    await userEvent.click(await screen.findByRole("button", { name: "Вернуть бадди" }));
    expect(screen.getByRole("button", { name: "Вернуть с комментарием" })).toBeDisabled();
    const reason = screen.getByLabelText("Что нужно доработать");
    expect(screen.getByText("Что нужно доработать").tagName).toBe("LABEL");
    expect(screen.getByText("Что нужно доработать")).toHaveAttribute("for", reason.id);
    await userEvent.type(reason, "Уточни сроки");
    await userEvent.click(screen.getByRole("button", { name: "Вернуть с комментарием" }));
    expect(await screen.findByText("Замечания руководителя")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Утвердить план" })).not.toBeInTheDocument();
    expect(JSON.parse(String(fetch.mock.calls[1][1]?.body))).toEqual({ version: 7, comment: "Уточни сроки" });
    await userEvent.click(screen.getByRole("button", { name: "История плана" }));
    await userEvent.click(screen.getByRole("button", { name: /Редакция 2 · Возвращён бадди/ }));
    expect(screen.getByText(/срок 12.11.2026 → 16.11.2026/)).toBeInTheDocument();
    expect(screen.getByText("Снимок: 2 шагов")).toBeInTheDocument();
  });

  it("при конфликте перечитывает план и не отправляет устаревшие правки повторно", async () => {
    const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(json(editing))
      .mockResolvedValueOnce(json({ error: { message: "План изменился. Обнови его." } }, 409))
      .mockResolvedValueOnce(json({ ...editing, version: 9, comment: "Правки другой вкладки" }));
    open();
    await screen.findByLabelText("Комментарий руководителю");
    fireEvent.change(screen.getByLabelText("Комментарий руководителю"), { target: { value: "Мои правки" } });
    await userEvent.click(screen.getByRole("button", { name: "Сохранить правки" }));
    expect(await screen.findByText("План изменился. Обнови его.")).toBeInTheDocument();
    expect(screen.getByLabelText("Комментарий руководителю")).toHaveValue("Правки другой вкладки");
    expect(screen.getByRole("button", { name: "Сохранить правки" })).toBeDisabled();
    expect(fetch.mock.calls).toHaveLength(3);
  });

  it("отменяет новую редакцию, оставляя действующую", async () => {
    const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(json(editing)).mockResolvedValueOnce(json({ ...base, version: 3 }));
    open();
    await userEvent.click(await screen.findByRole("button", { name: "Отменить редакцию" }));
    expect(await screen.findByRole("button", { name: "Изменить действующий план" })).toBeInTheDocument();
    expect(fetch.mock.calls[1][0]).toBe("/api/plans/u-sokolov/revisions/cancel");
    expect(JSON.parse(String(fetch.mock.calls[1][1]?.body)).version).toBe(2);
  });
});
