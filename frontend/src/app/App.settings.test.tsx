// Настройки (SCR-I8): стажёр меняет режим подсказок, видимость вопросов для бадди, напоминания и «не беспокоить»;
// у руководителя — только общие настройки. Данные — настоящие ответы backend (src/test/fixtures.json).
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mockApi, type Override } from "@/test/mockApi";
import App from "./App";

const openSettings = async (login: string, override?: Override) => {
  localStorage.setItem("naparnik.login", login);
  localStorage.setItem("naparnik.theme", "light");
  const api = mockApi(override);
  render(<App />);
  await userEvent.click(await screen.findByRole("button", { name: "Профиль и тестовые учётки" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: /Настройки/ }));
  const dialog = await screen.findByRole("dialog", { name: "Настройки" });
  await within(dialog).findByText("Рабочие часы");
  return { api, dialog };
};
const lastPatch = (api: ReturnType<typeof mockApi>) => api.calls.filter((c) => c.method === "PATCH" && c.path === "/api/settings").at(-1)?.body;

beforeEach(() => localStorage.clear());
afterEach(() => vi.unstubAllGlobals());

describe("настройки стажёра", () => {
  it("Полина: текст вопросов скрыт от бадди — включает, меняет режим и напоминания", async () => {
    const { api, dialog } = await openSettings("intern.egorova");
    const share = within(dialog).getByRole("checkbox", { name: /Показывать бадди текст/ });
    expect(share).not.toBeChecked();
    expect(within(dialog).getByRole("radio", { name: /Только дедлайны и встречи/ })).toBeChecked();
    expect(within(dialog).getByText("10:00-17:00")).toBeInTheDocument();

    await userEvent.click(share);
    await waitFor(() => expect(lastPatch(api)).toEqual({ question_text_visible_to_buddy: true }));
    expect(await within(dialog).findByRole("status")).toHaveTextContent("Сохранено");
    await waitFor(() => expect(within(dialog).getByRole("checkbox", { name: /Показывать бадди текст/ })).toBeChecked());

    await userEvent.click(within(dialog).getByRole("radio", { name: /План на день/ }));
    await waitFor(() => expect(lastPatch(api)).toEqual({ proactivity: "daily_plan" }));

    await userEvent.click(within(dialog).getByRole("checkbox", { name: "За день" }));
    await waitFor(() => expect(lastPatch(api)).toEqual({ reminders: ["1h", "1d"] }));
    await userEvent.click(within(dialog).getByRole("checkbox", { name: "За час" }));
    await waitFor(() => expect(lastPatch(api)).toEqual({ reminders: ["1d"] }));
  });

  it("«не беспокоить» включается на время и выключается; Telegram открывается из настроек", async () => {
    const { api, dialog } = await openSettings("intern.sokolov");
    await userEvent.click(within(dialog).getByRole("button", { name: "На 3 часа" }));
    await waitFor(() => expect(lastPatch(api)).toEqual({ quiet_hours: 3 }));
    expect(await within(dialog).findByText(/Включено до/)).toBeInTheDocument();
    await userEvent.click(within(dialog).getByRole("button", { name: "Выключить" }));
    await waitFor(() => expect(lastPatch(api)).toEqual({ quiet_hours: 0 }));
    expect(await within(dialog).findByRole("button", { name: "На час" })).toBeInTheDocument();
    await userEvent.click(within(dialog).getByRole("button", { name: "Подключить или отключить бота" }));
    expect(await screen.findByRole("dialog", { name: "Уведомления в Telegram" })).toBeInTheDocument();
  });

  it("ошибка сохранения видна, настройки не теряются", async () => {
    const { dialog } = await openSettings("intern.sokolov", (c) => (c.method === "PATCH" && c.path === "/api/settings"
      ? { status: 503, body: { error: { message: "Не удалось сохранить настройки. Повтори." } } } : undefined));
    await userEvent.click(within(dialog).getByRole("radio", { name: /Выключено/ }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Не удалось сохранить настройки");
    expect(within(dialog).getByRole("radio", { name: /План на день/ })).toBeChecked();
    await userEvent.click(within(dialog).getByRole("button", { name: "Готово" }));
  });
});

describe("настройки руководителя", () => {
  it("только напоминания, «не беспокоить» и часы — без режима подсказок и видимости для бадди", async () => {
    const { dialog } = await openSettings("lead.kovalev");
    expect(within(dialog).queryByText("Как Напарник подсказывает")).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("checkbox", { name: /Показывать бадди текст/ })).not.toBeInTheDocument();
    expect(within(dialog).getByRole("checkbox", { name: "За час" })).toBeChecked();
  });

  it("сбой загрузки настроек — понятная ошибка", async () => {
    const { dialog } = await openSettings("lead.kovalev", (c) => (c.path === "/api/settings" ? { status: 503, body: { error: { message: "Хранилище недоступно." } } } : undefined))
      .catch(async () => ({ dialog: await screen.findByRole("dialog", { name: "Настройки" }) }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Хранилище недоступно");
  });
});
