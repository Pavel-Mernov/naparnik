import type { ReactElement } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider } from "@mui/material";
import { describe, expect, it, vi } from "vitest";
import { makeTheme } from "@/shared/theme";
import type { Profile } from "@/shared/api";
import { ProfileDialog, shrinkImage } from "./ProfileDialog";

const intern: Profile = {
  id: "u-sokolov", name: "Артём Соколов", initials: "АС", photo: null, status: "Стажёр", position: "Стажёр-аналитик",
  department: "Аналитика Авито", phone: "+7 900 000-00-01", email: "intern.sokolov@naparnik.example",
  start_date: "2026-11-02", week: 2, buddy: "Мария Белова", lead: "Ольга Лебедева", can_edit: true,
};

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

const wrap = (ui: ReactElement) => render(<ThemeProvider theme={makeTheme("light")}>{ui}</ThemeProvider>);

describe("ProfileDialog", () => {
  it("shows the intern card with contacts and internship details", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(json({ ok: true, profile: intern }));
    wrap(<ProfileDialog open login="intern.sokolov" onClose={vi.fn()} />);
    expect(await screen.findByRole("heading", { name: "Артём Соколов" })).toBeInTheDocument();
    expect(fetchMock.mock.calls[0][0]).toBe("/api/profile");
    expect(new Headers(fetchMock.mock.calls[0][1]?.headers).get("X-User")).toBe("intern.sokolov");
    expect(screen.getByText("Стажёр")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "+7 900 000-00-01" })).toHaveAttribute("href", "tel:+79000000001");
    expect(screen.getByRole("link", { name: intern.email })).toHaveAttribute("href", `mailto:${intern.email}`);
    expect(screen.getByText(/2-я неделя/)).toBeInTheDocument();
    expect(screen.getByText("Мария Белова")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Загрузить фото" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Убрать фото" })).not.toBeInTheDocument();
  });

  it("opens someone else's card read-only", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(json({ ok: true, profile: { ...intern, can_edit: false } }));
    wrap(<ProfileDialog open login="buddy.belova" userId="u-sokolov" onClose={vi.fn()} />);
    await screen.findByRole("heading", { name: "Артём Соколов" });
    expect(fetchMock.mock.calls[0][0]).toBe("/api/profiles/u-sokolov");
    expect(screen.queryByRole("button", { name: "Загрузить фото" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Изменить" })).not.toBeInTheDocument();
    expect(screen.getByText("Профиль сотрудника")).toBeInTheDocument();
  });

  it("validates phone with hint, blocks letters, and saves 11-digit +7 number", async () => {
    const onChanged = vi.fn();
    const fetchMock = vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(json({ ok: true, profile: intern }))
      .mockResolvedValueOnce(json({ ok: true, profile: { ...intern, phone: "+7 912 345-67-89" } }));
    wrap(<ProfileDialog open login="intern.sokolov" onClose={vi.fn()} onChanged={onChanged} />);
    await userEvent.click(await screen.findByRole("button", { name: "Изменить" }));
    const field = screen.getByLabelText("Рабочий телефон");
    expect(screen.getByText("+7")).toBeInTheDocument();
    expect(screen.getByText(/Введите корректный номер телефона например/)).toBeInTheDocument();
    await userEvent.clear(field);
    await userEvent.type(field, "abc123");
    expect(field).toHaveValue("123");
    expect(screen.getByRole("button", { name: "Сохранить" })).toBeDisabled();
    await userEvent.clear(field);
    await userEvent.type(field, "9123456789");
    expect(await screen.findByText(/успешно валидирован/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Сохранить" }));
    expect(await screen.findByRole("link", { name: "+7 912 345-67-89" })).toBeInTheDocument();
    expect(fetchMock.mock.calls[1][1]).toMatchObject({ method: "PATCH", body: JSON.stringify({ phone: "+7 912 345-67-89" }) });
    expect(onChanged).toHaveBeenCalledWith(expect.objectContaining({ phone: "+7 912 345-67-89" }));
  });

  it("uploads a small photo as is and can remove it", async () => {
    const withPhoto = { ...intern, photo: { url: "/api/profiles/u-sokolov/photo?v=1", updated_at: "1" } };
    const createURL = vi.fn(() => "blob:photo");
    Object.assign(URL, { createObjectURL: createURL, revokeObjectURL: vi.fn() });
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
      const url = String(input);
      if (url === "/api/profile") return json({ ok: true, profile: intern });
      if (url === "/api/profile/photo" && init?.method === "PUT") return json({ ok: true, profile: withPhoto });
      if (url === "/api/profile/photo" && init?.method === "DELETE") return json({ ok: true, profile: intern });
      if (url.startsWith("/api/profiles/u-sokolov/photo")) return new Response("x", { headers: { "Content-Type": "image/png" } });
      return json({}, 404);
    });
    wrap(<ProfileDialog open login="intern.sokolov" onClose={vi.fn()} />);
    await screen.findByRole("button", { name: "Загрузить фото" });
    const file = new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47])], "me.png", { type: "image/png" });
    await userEvent.upload(screen.getByTestId("photo-input"), file);
    expect(await screen.findByRole("button", { name: "Убрать фото" })).toBeInTheDocument();
    const put = fetchMock.mock.calls.find(([u, i]) => u === "/api/profile/photo" && i?.method === "PUT")!;
    expect(put[1]?.body).toBe(file);
    await waitFor(() => expect(createURL).toHaveBeenCalled());
    const photoCall = fetchMock.mock.calls.find(([u]) => String(u).startsWith("/api/profiles/u-sokolov/photo"))!;
    expect(new Headers(photoCall[1]?.headers).get("X-User")).toBe("intern.sokolov");
    await userEvent.click(screen.getByRole("button", { name: "Убрать фото" }));
    expect(await screen.findByRole("button", { name: "Загрузить фото" })).toBeInTheDocument();
  });

  it("shows an error when the profile is unavailable", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(json({ ok: false, error: { code: "not_found", message: "Профиль не найден." } }, 404));
    wrap(<ProfileDialog open login="intern.sokolov" userId="u-nobody" onClose={vi.fn()} />);
    expect(await screen.findByText("Профиль не найден.")).toBeInTheDocument();
  });
});

describe("shrinkImage", () => {
  it("rejects unsupported types and keeps small files untouched", async () => {
    await expect(shrinkImage(new File(["<svg/>"], "x.svg", { type: "image/svg+xml" }))).rejects.toThrow(/JPEG, PNG или WebP/);
    const small = new File(["abc"], "a.jpg", { type: "image/jpeg" });
    await expect(shrinkImage(small)).resolves.toBe(small);
  });
});
