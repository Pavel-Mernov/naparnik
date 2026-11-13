// Маскот машет письмом только на новое важное (08-DESIGN §2.4): открыл чат или колокольчик — успокоился;
// просмотренное помнится после перезагрузки, новое важное — снова письмо.
import { render, screen, waitFor } from "@testing-library/react";
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
