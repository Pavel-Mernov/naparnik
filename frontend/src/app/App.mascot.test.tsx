// Маскот машет письмом только на новое важное (08-DESIGN §2.4): открыл чат или колокольчик — успокоился;
// просмотренное помнится после перезагрузки, новое важное — снова письмо.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import fixtures from "@/test/fixtures.json";
import { mockApi, type Override } from "@/test/mockApi";
import App from "./App";

const fx = fixtures as Record<string, Record<string, unknown>>;
const mascot = () => screen.getByRole("button", { name: "Открыть чат с Напарником" });
const start = (override?: Override) => {
  localStorage.setItem("naparnik.login", "intern.sokolov");
  localStorage.setItem("naparnik.theme", "light");
  localStorage.setItem("naparnik.mascot.greeting", String(Date.now())); // приветствие уже было сегодня
  mockApi(override);
  return render(<App />);
};

beforeEach(() => localStorage.clear());
afterEach(() => vi.unstubAllGlobals());

describe("маскот и важное", () => {
  it("машет письмом про непросмотренное, после колокольчика — спокоен, и после перезагрузки тоже", async () => {
    const first = start();
    await screen.findByRole("heading", { name: /Даниил, на сегодня/ });
    // ответ Ольги и просроченный шаг — ещё не видел: письмо
    await waitFor(() => expect(mascot().getAttribute("data-mood")).toMatch(/^(has_answer|reminder)$/));
    await userEvent.click(screen.getByRole("button", { name: /^Уведомления:/ }));
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(mascot()).toHaveAttribute("data-mood", "idle"), { timeout: 3000 });
    first.unmount();
    vi.unstubAllGlobals();

    start();
    await screen.findByRole("heading", { name: /Даниил, на сегодня/ });
    await waitFor(() => expect(mascot()).toHaveAttribute("data-mood", "idle"), { timeout: 3000 });
  });

  it("новое важное после просмотра — снова письмо; открыл чат — успокоился", async () => {
    localStorage.setItem("naparnik.mascot.seen.intern.sokolov", JSON.stringify(["o-s-04", "ans-Q-101", "r-ABS-1042"]));
    const q = (fx["intern.sokolov"]["/api/questions"] as { questions: Array<Record<string, unknown>> }).questions;
    const answered = { questions: [...q, { ...q[0], id: "Q-777", status: "Answered", answer: "Свежий ответ" }] };
    start((c) => (c.method === "GET" && c.path === "/api/questions" ? { body: answered } : undefined));
    await screen.findByRole("heading", { name: /Даниил, на сегодня/ });
    await waitFor(() => expect(mascot()).toHaveAttribute("data-mood", "has_answer"), { timeout: 3000 });
    await userEvent.click(mascot());
    await userEvent.click(await screen.findByRole("button", { name: "Закрыть чат" }));
    await waitFor(() => expect(mascot()).toHaveAttribute("data-mood", "idle"), { timeout: 3000 });
  });
});

// 08-DESIGN §2.5: перетаскивание к краю, меню по правому клику; всё запоминается для учётки.
describe("маскот: место и меню", () => {
  const place = () => JSON.parse(localStorage.getItem("naparnik.mascot.place.intern.sokolov") ?? "null");

  it("перетащили влево — прилип к левому краю, чат не открылся, место помнится после перезагрузки", async () => {
    const first = start();
    await screen.findByRole("heading", { name: /Даниил, на сегодня/ });
    const m = mascot();
    fireEvent.pointerDown(m, { button: 0, clientX: 1000, clientY: 700, pointerId: 1 });
    fireEvent.pointerMove(m, { clientX: 600, clientY: 500, pointerId: 1 });
    expect(m.style.left).not.toBe(""); // во время перетаскивания идёт за курсором
    fireEvent.pointerMove(m, { clientX: 40, clientY: 300, pointerId: 1 });
    fireEvent.pointerUp(m, { clientX: 40, clientY: 300, pointerId: 1 });
    fireEvent.click(m);
    expect(screen.queryByRole("dialog", { name: /Напарник/ })).not.toBeInTheDocument();
    expect(m.getAttribute("data-edge")).toBe("left");
    expect(place()).toMatchObject({ edge: "left", minimized: false });
    first.unmount();
    start();
    await screen.findByRole("heading", { name: /Даниил, на сегодня/ });
    expect(mascot().getAttribute("data-edge")).toBe("left");
  });

  it("правый клик: свернуть до точки и развернуть кликом; скрыть на час и вернуть из меню профиля", async () => {
    start();
    await screen.findByRole("heading", { name: /Даниил, на сегодня/ });
    fireEvent.contextMenu(mascot(), { clientX: 900, clientY: 600 });
    await userEvent.click(within(await screen.findByRole("menu")).getByRole("menuitem", { name: "Свернуть" }));
    const dot = await screen.findByRole("button", { name: "Развернуть Напарника" });
    expect(place().minimized).toBe(true);
    await userEvent.click(dot);
    expect(await screen.findByRole("button", { name: "Открыть чат с Напарником" })).toBeInTheDocument();

    fireEvent.contextMenu(mascot(), { clientX: 900, clientY: 600 });
    await userEvent.click(within(await screen.findByRole("menu")).getByRole("menuitem", { name: "Скрыть на час" }));
    await waitFor(() => expect(screen.queryByRole("button", { name: "Открыть чат с Напарником" })).not.toBeInTheDocument());
    expect(place().hiddenUntil).toBeGreaterThan(Date.now());
    await userEvent.click(screen.getByRole("button", { name: /Даниил Соколов|Профиль|Аккаунт/ }));
    await userEvent.click(await screen.findByRole("menuitem", { name: /Показать Напарника/ }));
    expect(await screen.findByRole("button", { name: "Открыть чат с Напарником" })).toBeInTheDocument();
  });
});
