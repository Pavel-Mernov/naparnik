import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import fixtures from "@/test/fixtures.json";
import { mockApi, type ApiCall } from "@/test/mockApi";
import App from "./App";

const fx = fixtures as Record<string, Record<string, unknown>>;
const text = "📚 Андрей Ковалёв назначил(а) тебя владельцем базы знаний отдела.";
const note = { id: "N-700", type: "kb_owner", text: text + "\nОткрой поддержку базы знаний.", view: "knowledge", read: false };
const mount = (login: string) => { localStorage.setItem("naparnik.login", login); render(<App />); };
const emptyNotesSources = (c: ApiCall) => {
  if (c.path === "/api/questions") return { body: { questions: [] } };
  if (c.path === "/api/kb/drafts") return { body: { drafts: [] } };
  return undefined;
};
beforeEach(() => localStorage.clear());

describe("BR-34: уведомление новому владельцу базы", () => {
  it("передача от Ольги Екатерине: уведомление видно под учёткой бадди и ведёт в поддержку базы", async () => {
    let owner = "u-smirnova", assigned = false;
    const api = mockApi((c) => {
      if (c.path === "/api/kb/owner" && c.method === "PUT") { owner = (c.body as { user_id: string }).user_id; assigned = true; }
      if (c.path === "/api/kb/owner") return { body: { owner_id: owner, default_id: "u-kovalev", can_change: c.user === "lead.kovalev", is_owner: c.user === "buddy.belova" && owner === "u-belova" } };
      if (c.path === "/api/notifications") return { body: { notifications: assigned && c.user === "buddy.belova" ? [note] : [] } };
      if (c.path === "/api/kb/maintenance" && c.user === "buddy.belova") return { body: { ...(fx["lead.kovalev"]["/api/kb/maintenance"] as object), owner_id: owner } };
      return emptyNotesSources(c);
    });
    mount("lead.kovalev");
    await userEvent.click(await screen.findByRole("button", { name: "Передать другому" }));
    const dialog = await screen.findByRole("dialog", { name: "Владелец базы знаний" });
    await userEvent.click(within(dialog).getByRole("combobox"));
    await userEvent.click(await screen.findByRole("option", { name: /Екатерина Белова/ }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Передать" }));
    await waitFor(() => expect(api.writesTo("/api/kb/owner")[0]?.body).toEqual({ user_id: "u-belova" }));
    cleanup(); mount("buddy.belova");
    await screen.findByRole("heading", { name: /У стажёров|есть просрочки/ });
    const bell = await screen.findByRole("button", { name: "Уведомления: 1" });
    expect(bell.querySelector(".MuiBadge-badge")).toHaveTextContent("1");
    await userEvent.click(bell);
    await userEvent.click(await screen.findByText(text, { exact: true }));
    expect(await screen.findByText("Поддержка базы знаний", { exact: true })).toBeInTheDocument();
    await waitFor(() => expect(api.calls.some((c) => c.path === "/api/kb/maintenance" && c.user === "buddy.belova")).toBe(true));
    expect(JSON.parse(localStorage.getItem("naparnik.mascot.seen.buddy.belova") ?? "[]")).toContain(note.id);
    expect(bell.querySelector(".MuiBadge-badge")).toHaveClass("MuiBadge-invisible");
  });
  it("прочитанное на сервере назначение не появляется заново", async () => {
    mockApi((c) => c.path === "/api/notifications" ? { body: { notifications: [{ ...note, read: true }] } } : emptyNotesSources(c));
    mount("buddy.belova");
    await screen.findByRole("heading", { name: /У стажёров|есть просрочки/ });
    await userEvent.click(screen.getByRole("button", { name: /^Уведомления:/ }));
    expect(await screen.findByText("Всё спокойно — новых событий нет.")).toBeInTheDocument();
    expect(screen.queryByText(text)).not.toBeInTheDocument();
  });
});
