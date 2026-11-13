import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider } from "@mui/material";
import { describe, expect, it, vi } from "vitest";
import { makeTheme } from "@/shared/theme";
import { ChatDrawer, type ChatMessage, type Lookup } from "./ChatDrawer";

const look: Lookup = { userName: (id) => id, systemTitle: (id) => id, stepTitle: (id) => id };

const drawer = (messages: ChatMessage[], onReset?: () => void) => render(<ThemeProvider theme={makeTheme("light")}>
  <ChatDrawer open onClose={vi.fn()} text="" setText={vi.fn()} messages={messages} loading={false} onSend={vi.fn()}
    login="intern.sokolov" look={look} onAction={vi.fn()} onReset={onReset} />
</ThemeProvider>);

const absence = (status?: "pending" | "confirmed" | "cancelled") => ({
  id: "DRAFT-1", tool: "create_absence_request", args: { type: "day_off", date: "2026-11-13" }, version: 1, status,
  result: status === "confirmed" ? { receipt: { id: "ABS-1042", status: "pending", approver: "Андрей Ковалёв" } } : undefined,
});
const restored = (status?: "pending" | "confirmed" | "cancelled"): ChatMessage[] => [
  { role: "assistant", text: "Привет!" },
  { role: "user", text: "Нужен отгул в пятницу" },
  { role: "assistant", text: "Черновик готов.", reply: { text: "Черновик готов.", drafts: [absence(status)], sources: [], degraded: false, retry: false } },
];

// История чата после перезагрузки: черновик — в том состоянии, в каком он на сервере.
describe("ChatDrawer — история", () => {
  it("shows a confirmed draft as a receipt, not as a new confirmation", () => {
    drawer(restored("confirmed"));
    expect(screen.getByText(/Готово: ABS-1042 · на согласовании · Андрей Ковалёв/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Подтвердить" })).not.toBeInTheDocument();
  });

  it("shows a cancelled draft as cancelled", () => {
    drawer(restored("cancelled"));
    expect(screen.getByText("Отменено — ничего не отправлено.")).toBeInTheDocument();
  });

  it("keeps a pending draft confirmable", () => {
    drawer(restored("pending"));
    expect(screen.getByRole("button", { name: "Подтвердить" })).toBeInTheDocument();
  });

  it("offers a new conversation only when there is something to clear", async () => {
    const onReset = vi.fn();
    const { unmount } = drawer([{ role: "assistant", text: "Привет!" }], onReset);
    expect(screen.queryByRole("button", { name: "Новый разговор" })).not.toBeInTheDocument();
    unmount();
    drawer(restored("pending"), onReset);
    await userEvent.click(screen.getByRole("button", { name: "Новый разговор" }));
    expect(onReset).toHaveBeenCalled();
  });
});
