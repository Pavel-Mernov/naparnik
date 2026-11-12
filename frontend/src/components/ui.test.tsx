import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider } from "@mui/material";
import { describe, expect, it, vi } from "vitest";
import { makeTheme } from "../theme";
import { AiTag, Block, Empty, PageHead, Row, Shapes, StatusTag, TestLabel } from "./ui";

const ui = (node: React.ReactNode) => render(<ThemeProvider theme={makeTheme("light")}>{node}</ThemeProvider>);

describe("shared UI", () => {
  it("renders status, headings, empty states and labels", () => {
    ui(<><StatusTag status="Done" /><StatusTag status="unknown" label="Custom" /><PageHead context="context" title="Title" action={<button>Action</button>} />
      <Block title="Block" aside="Aside" tone="surface"><span>Body</span></Block><Empty text="Empty" action={<button>Fix</button>} />
      <Shapes variant={4} /><AiTag /><AiTag>AI</AiTag><TestLabel /></>);
    expect(screen.getByText("Готово")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Title" })).toBeInTheDocument();
    expect(screen.getByText("Empty")).toBeInTheDocument();
    expect(screen.getByText("тестовая инструкция, не регламент Авито")).toBeInTheDocument();
  });

  it("activates clickable rows with click and Enter", async () => {
    const onClick = vi.fn();
    ui(<><Row title="Interactive" sub="Details" end="End" start="Start" onClick={onClick} /><Row title="Static" /></>);
    await userEvent.click(screen.getByRole("button", { name: /Interactive/ }));
    fireEvent.keyDown(screen.getByRole("button", { name: /Interactive/ }), { key: "Enter" });
    fireEvent.keyDown(screen.getByRole("button", { name: /Interactive/ }), { key: "Escape" });
    expect(onClick).toHaveBeenCalledTimes(2);
  });
});
