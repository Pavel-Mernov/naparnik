import type { ReactElement } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider } from "@mui/material";
import { describe, expect, it, vi } from "vitest";
import { makeTheme } from "@/shared/theme";
import type { PlanDraft } from "@/shared/api";
import { PlanDraftDialog } from "./PlanDraftDialog";

const base: PlanDraft = {
  version: 1, revision: 1, active_revision: 0, review_status: "review", changes: [], history: [], can_submit: false, can_return: false, can_start: false, can_cancel: false,
  intern_id: "u-egorova", intern: "Полина Егорова", status: "draft", created_at: "2026-11-11T09:00:00+03:00",
  auto_activate_at: "2026-11-13T09:00:00+03:00", can_edit: true, can_approve: false,
  stages: [{ id: "st-w2", title: "Неделя 2", order: 2 }, { id: "st-w1", title: "Неделя 1", order: 1 }],
  steps: [
    { id: "s-01", title: "Познакомься с командой", stage: "st-w1", owner: "intern", due_date: "2026-12-09" },
    { id: "s-05", title: "Добавь стажёра в чаты", stage: "st-w1", owner: "buddy", due_date: "2026-12-08" },
    { id: "s-09", title: "Разбери дашборд", stage: "st-w2", owner: "intern", due_date: "2026-12-15" },
  ],
};

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
const wrap = (ui: ReactElement) => render(<ThemeProvider theme={makeTheme("light")}>{ui}</ThemeProvider>);

describe("PlanDraftDialog", () => {
  it("buddy edits due dates, removes a step and sends only the changes", async () => {
    const onDone = vi.fn();
    const fetchMock = vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(json(base))
      .mockResolvedValueOnce(json({ ...base, comment: "Без Tableau", edited_by: "Екатерина Белова" }));
    wrap(<PlanDraftDialog open login="buddy.belova" internId="u-egorova" onClose={vi.fn()} onDone={onDone} />);
    expect(await screen.findByRole("heading", { name: "Полина Егорова" })).toBeInTheDocument();
    expect(fetchMock.mock.calls[0][0]).toBe("/api/plans/u-egorova/draft");
    expect(screen.getByText("Черновик")).toBeInTheDocument();
    // этапы по порядку, шаги внутри
    const headings = screen.getAllByRole("heading", { level: 3 }).map((h) => h.textContent);
    expect(headings).toEqual(["Неделя 1", "Неделя 2"]);
    const save = screen.getByRole("button", { name: "Сохранить правки" });
    expect(save).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Утвердить план" })).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Срок: Познакомься с командой"), { target: { value: "2026-12-11" } });
    await userEvent.click(screen.getAllByRole("button", { name: "Убрать" })[1]);
    expect(screen.getByText(/убран из плана/)).toBeInTheDocument();
    expect(screen.getByText("2 из 3 шагов в плане")).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText("Комментарий руководителю"), "Без Tableau");
    await userEvent.click(save);

    await waitFor(() => expect(onDone).toHaveBeenCalledWith(expect.stringMatching(/Правки сохранены/)));
    const [url, init] = fetchMock.mock.calls[1];
    expect(url).toBe("/api/plans/u-egorova/draft");
    expect(init?.method).toBe("PATCH");
    expect(JSON.parse(String(init?.body))).toEqual({
      steps: [{ due_date: "2026-12-11", step_id: "s-01" }, { removed: true, step_id: "s-05" }],
      comment: "Без Tableau",
      version: 1,
    });
    expect(await screen.findByText("Без Tableau")).toBeInTheDocument();
  });

  it("reverting a change leaves nothing to save", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(json(base));
    wrap(<PlanDraftDialog open login="buddy.belova" internId="u-egorova" onClose={vi.fn()} onDone={vi.fn()} />);
    await screen.findByRole("heading", { name: "Полина Егорова" });
    await userEvent.click(screen.getAllByRole("button", { name: "Убрать" })[0]);
    expect(screen.getByRole("button", { name: "Сохранить правки" })).toBeEnabled();
    await userEvent.click(screen.getByRole("button", { name: "Вернуть" }));
    expect(screen.getByRole("button", { name: "Сохранить правки" })).toBeDisabled();
  });

  it("lead approves an auto-activated plan read-only", async () => {
    const onDone = vi.fn(), onClose = vi.fn();
    const fetchMock = vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(json({ ...base, status: "auto_activated", can_edit: false, can_approve: true, auto_activate_at: undefined }))
      .mockResolvedValueOnce(json({ ...base, status: "approved" }));
    wrap(<PlanDraftDialog open login="lead.kovalev" internId="u-egorova" onClose={onClose} onDone={onDone} />);
    expect(await screen.findByText("Не утверждён")).toBeInTheDocument();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Убрать" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Утвердить план" }));
    await waitFor(() => expect(onDone).toHaveBeenCalled());
    expect(fetchMock.mock.calls[1][0]).toBe("/api/plans/u-egorova/approve");
    expect(fetchMock.mock.calls[1][1]?.method).toBe("POST");
    expect(onDone).toHaveBeenCalledWith(expect.stringMatching(/План утверждён/));
  });

  it("shows backend errors and keeps the dialog open", async () => {
    vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(json(base))
      .mockResolvedValueOnce(json({ ok: false, error: { code: "validation", message: "Срок — рабочий день в формате ГГГГ-ММ-ДД." } }, 422));
    wrap(<PlanDraftDialog open login="buddy.belova" internId="u-egorova" onClose={vi.fn()} onDone={vi.fn()} />);
    await screen.findByRole("heading", { name: "Полина Егорова" });
    fireEvent.change(screen.getByLabelText("Срок: Разбери дашборд"), { target: { value: "2026-12-12" } });
    await userEvent.click(screen.getByRole("button", { name: "Сохранить правки" }));
    expect(await screen.findByText("Срок — рабочий день в формате ГГГГ-ММ-ДД.")).toBeInTheDocument();
  });

  it("approved plan has no actions", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(json({ ...base, status: "approved", approved_by: "Андрей Ковалёв", can_edit: false }));
    wrap(<PlanDraftDialog open login="lead.kovalev" internId="u-egorova" onClose={vi.fn()} onDone={vi.fn()} />);
    expect(await screen.findByText("Утвердил(а) Андрей Ковалёв")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Утвердить|Сохранить/ })).not.toBeInTheDocument();
  });
});
