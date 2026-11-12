import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider } from "@mui/material";
import { describe, expect, it, vi } from "vitest";
import { makeTheme } from "../theme";
import { PersonDialog } from "./PersonDialog";

const people = [{ id: "u1", login: "one", name: "Иван Иванов", position: "Бадди", roles: ["buddy"] }];

describe("PersonDialog", () => {
  it("selects a person and maps automatic routing to an empty id", async () => {
    const onPick = vi.fn();
    const { rerender } = render(<ThemeProvider theme={makeTheme("light")}><PersonDialog open title="Кому" text="Выбери" people={people} allowAuto confirm="Назначить" onClose={vi.fn()} onPick={onPick} /></ThemeProvider>);
    const button = screen.getByRole("button", { name: "Назначить" });
    expect(button).toBeDisabled();
    await userEvent.click(screen.getByRole("combobox"));
    await userEvent.click(screen.getByText(/Иван Иванов/));
    await userEvent.click(button);
    expect(onPick).toHaveBeenCalledWith("u1");
    rerender(<ThemeProvider theme={makeTheme("light")}><PersonDialog open title="Кому" text="Выбери" people={people} allowAuto confirm="Назначить" onClose={vi.fn()} onPick={onPick} /></ThemeProvider>);
    await userEvent.click(screen.getByRole("combobox"));
    await userEvent.click(screen.getByText(/Не знаю/));
    await userEvent.click(screen.getByRole("button", { name: "Назначить" }));
    expect(onPick).toHaveBeenLastCalledWith("");
  });
});
