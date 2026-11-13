import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider } from "@mui/material";
import { describe, expect, it, vi } from "vitest";
import { makeTheme } from "@/shared/theme";
import type { ChatTurn } from "@/shared/api";
import { AskedList } from "./QuestionsPage";

const turn = (id: string, question: string, reply: Partial<ChatTurn["reply"]>): ChatTurn => ({
  id, conversation: 0, question, at: "2026-11-11T10:30:00+03:00",
  reply: { text: "Ответ", drafts: [], sources: [], degraded: false, retry: false, ...reply },
});

// «Спрашивал у Напарника»: вопросы не пропадают, новые сверху, ответ раскрывается.
describe("AskedList", () => {
  it("lists questions newest first with what happened to them", () => {
    render(<ThemeProvider theme={makeTheme("light")}><AskedList asked={[
      turn("1", "Что такое витрина?", { sources: [{ page_id: "kb-03", title: "Витрины DWH", author: "Михаил Орлов", updated_at: "2026-10-28" }] }),
      turn("2", "Нужен отгул в пятницу", { drafts: [{ id: "D1", tool: "create_absence_request", args: {}, status: "confirmed" }] }),
    ]} /></ThemeProvider>);
    const items = screen.getAllByRole("button");
    expect(items[0]).toHaveTextContent("Нужен отгул в пятницу");
    expect(items[0]).toHaveTextContent("Напарник оформил — подтверждено");
    expect(items[1]).toHaveTextContent("Ответ из базы: Витрины DWH");
  });

  it("expands the answer with its source and opens the chat", async () => {
    const onOpenChat = vi.fn();
    render(<ThemeProvider theme={makeTheme("light")}><AskedList onOpenChat={onOpenChat} asked={[
      turn("1", "Что такое витрина?", { text: "Витрина — подготовленная таблица.", sources: [{ page_id: "kb-03", title: "Витрины DWH", author: "Михаил Орлов", updated_at: "2026-10-28" }] }),
    ]} /></ThemeProvider>);
    expect(screen.queryByText("Витрина — подготовленная таблица.")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /Что такое витрина/ }));
    expect(screen.getByText("Витрина — подготовленная таблица.")).toBeInTheDocument();
    expect(screen.getByText(/Источник: Витрины DWH · Михаил Орлов/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Открыть чат" }));
    expect(onOpenChat).toHaveBeenCalled();
  });
});
