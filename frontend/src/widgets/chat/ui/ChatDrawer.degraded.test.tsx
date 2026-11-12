import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider } from "@mui/material";
import { describe, expect, it, vi } from "vitest";
import { makeTheme } from "@/shared/theme";
import { ChatDrawer, type ChatMessage, type Lookup } from "./ChatDrawer";

const look: Lookup = { userName: (id) => id, systemTitle: (id) => id, stepTitle: (id) => id };

const drawer = (messages: ChatMessage[], onSend = vi.fn()) => {
  render(<ThemeProvider theme={makeTheme("light")}>
    <ChatDrawer open onClose={vi.fn()} text="" setText={vi.fn()} messages={messages} loading={false} onSend={onSend}
      login="intern.sokolov" look={look} onAction={vi.fn()} />
  </ThemeProvider>);
  return onSend;
};

// BR-70: модель недоступна — пометка режима и кнопки быстрых действий отправляют фразу в чат.
describe("ChatDrawer — упрощённый режим", () => {
  it("shows the degraded label and quick actions on the last answer", async () => {
    const onSend = drawer([
      { role: "user", text: "Что такое витрина?" },
      {
        role: "assistant", text: "Модель сейчас недоступна. Отвечаю в упрощённом режиме, по правилам.", retryText: "Что такое витрина?",
        reply: {
          text: "", drafts: [], sources: [], degraded: true, retry: true,
          actions: [{ label: "Спросить человека", text: "Не помогло, нужен человек: Что такое витрина?" }, { label: "Оформить отгул", text: "Нужен отгул" }],
        },
      },
    ]);
    expect(screen.getByText("Упрощённый режим · без модели")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Оформить отгул" }));
    expect(onSend).toHaveBeenCalledWith("Нужен отгул");
    expect(screen.getByRole("button", { name: "Повторить" })).toBeInTheDocument();
  });

  it("hides quick actions on older answers", () => {
    const reply = { text: "", drafts: [], sources: [], degraded: true, retry: false, actions: [{ label: "Оформить отгул", text: "Нужен отгул" }] };
    drawer([
      { role: "assistant", text: "Модель сейчас недоступна.", reply },
      { role: "user", text: "Нужен отгул" },
      { role: "assistant", text: "На какую дату нужен отгул?", reply: { ...reply, actions: [] } },
    ]);
    expect(screen.queryByRole("button", { name: "Оформить отгул" })).not.toBeInTheDocument();
    expect(screen.getAllByText("Упрощённый режим · без модели")).toHaveLength(2);
  });
});
