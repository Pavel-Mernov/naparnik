// Запоздавшие ответы после смены учётки (SC-13, SC-21): транспорт не отменяет запрос, ответ приходит,
// когда на экране уже другая учётка, — он не должен ни показаться, ни изменить её данные.
// Перенесено из прежнего браузерного теста scripts/test_role_ui.py.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import fixtures from "@/test/fixtures.json";
import { mockApi, type Override } from "@/test/mockApi";
import App from "./App";

const fx = fixtures as Record<string, Record<string, unknown>>;
const held = () => {
  let release!: () => void;
  const gate = new Promise<void>((r) => { release = r; });
  return { gate, release };
};
const start = (override: Override) => {
  localStorage.setItem("naparnik.login", "intern.sokolov");
  localStorage.setItem("naparnik.theme", "light");
  const api = mockApi(override);
  render(<App />);
  return api;
};
const switchTo = async (name: RegExp) => {
  await userEvent.click(await screen.findByRole("button", { name: "Профиль и тестовые учётки" }));
  await userEvent.click(await screen.findByRole("menuitem", { name }));
};

beforeEach(() => localStorage.clear());
afterEach(() => vi.unstubAllGlobals());

describe("запоздавший ответ старой учётки", () => {
  it("загрузка плана стажёра после переключения на руководителя не подменяет его экран", async () => {
    const { gate, release } = held();
    let arrived = false;
    start(async (c) => {
      if (c.path !== "/api/plans/u-sokolov" || c.user !== "intern.sokolov" || arrived) return undefined;
      arrived = true;
      await gate;
      return { body: fx["intern.sokolov"]["/api/plans/u-sokolov"] };
    });
    await waitFor(() => expect(arrived).toBe(true));
    await switchTo(/Андрей Ковалёв/);
    expect(await screen.findByText("Стажёры")).toBeInTheDocument();
    release();
    await new Promise((r) => setTimeout(r, 100));
    expect(screen.getByText("Стажёры")).toBeInTheDocument();
    expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
  });

  it("ответ чата старой учётки не появляется — даже после возврата A → B → A", async () => {
    const { gate, release } = held();
    let done = false;
    start(async (c) => {
      if (c.path !== "/api/chat/messages") return undefined;
      await gate;
      done = true;
      return { body: { text: "STALE-PRIVATE-REPLY", sources: [], drafts: [], degraded: false, retry: false } };
    });
    await screen.findByRole("heading", { name: /Даниил, на сегодня/ });
    await userEvent.click(screen.getByRole("button", { name: "Открыть чат с Напарником" }));
    await userEvent.type(screen.getByPlaceholderText("Спроси о команде или задачах"), "Личный вопрос старой учётки{Enter}");
    await userEvent.click(screen.getByRole("button", { name: "Закрыть чат" }));
    await switchTo(/Андрей Ковалёв/);
    await screen.findByText("Стажёры");
    await switchTo(/Даниил Соколов/);
    await screen.findByRole("heading", { name: /Даниил, на сегодня/ });
    await userEvent.click(screen.getByRole("button", { name: "Открыть чат с Напарником" }));
    release();
    await waitFor(() => expect(done).toBe(true));
    await new Promise((r) => setTimeout(r, 100));
    expect(screen.queryByText("STALE-PRIVATE-REPLY")).not.toBeInTheDocument();
    expect(screen.queryByText("Личный вопрос старой учётки")).not.toBeInTheDocument();
  });

  it("отметка шага уходит от имени стажёра, но её итог не показывается руководителю", async () => {
    const { gate, release } = held();
    let body: unknown, user = "";
    const api = start(async (c) => {
      if (!c.path.startsWith("/api/plan-steps/")) return undefined;
      body = c.body; user = c.user;
      await gate;
      return { body: { ok: true } };
    });
    await screen.findByRole("heading", { name: /Даниил, на сегодня/ });
    await userEvent.click(screen.getAllByRole("checkbox")[0]);
    await waitFor(() => expect(user).toBe("intern.sokolov"));
    expect(body).toEqual({ intern_id: "u-sokolov" });
    await switchTo(/Андрей Ковалёв/);
    await screen.findByText("Стажёры");
    release();
    await new Promise((r) => setTimeout(r, 100));
    expect(screen.queryByText("Шаг отмечен — так держать")).not.toBeInTheDocument();
    expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
    expect(within(screen.getByRole("navigation", { name: "Разделы" })).getByRole("button", { name: "Согласование" })).toBeInTheDocument();
    expect(api.writesTo("/api/plan-steps/")).toHaveLength(1);
  });
});
