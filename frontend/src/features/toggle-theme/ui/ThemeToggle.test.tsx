import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ThemeToggle } from "./ThemeToggle";

describe("ThemeToggle", () => {
  it("shows the moon in light mode and the sun in dark mode", () => {
    const { rerender } = render(<ThemeToggle mode="light" onToggle={vi.fn()} />);
    expect(screen.getByTestId("theme-icon-moon")).toHaveStyle({ opacity: "1" });
    expect(screen.getByTestId("theme-icon-sun")).toHaveStyle({ opacity: "0" });
    rerender(<ThemeToggle mode="dark" onToggle={vi.fn()} />);
    expect(screen.getByTestId("theme-icon-sun")).toHaveStyle({ opacity: "1" });
    expect(screen.getByTestId("theme-icon-moon")).toHaveStyle({ opacity: "0" });
  });

  it("toggles on click", async () => {
    const onToggle = vi.fn();
    render(<ThemeToggle mode="light" onToggle={onToggle} />);
    await userEvent.click(screen.getByRole("button", { name: "Сменить тему" }));
    expect(onToggle).toHaveBeenCalledOnce();
  });
});
