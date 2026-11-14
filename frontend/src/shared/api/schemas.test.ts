import { describe, expect, it, vi } from "vitest";
import fixtures from "@/test/fixtures.json";
import { checkShape, schemaFor } from "./schemas";

describe("проверка ответов сервера", () => {
  // Записанные ответы настоящего backend должны проходить схемы — иначе проверка сломала бы рабочий экран.
  it("все записанные ответы backend проходят схемы", () => {
    let checked = 0;
    for (const [user, paths] of Object.entries(fixtures as Record<string, Record<string, unknown>>))
      for (const [path, body] of Object.entries(paths)) {
        if (!schemaFor("GET", path)) continue;
        expect(checkShape("GET", path, body), `${user} ${path}`).toBeNull();
        checked++;
      }
    expect(checked).toBeGreaterThan(30);
  });

  it("ответ не той формы — понятная ошибка, детали в консоль", () => {
    const log = vi.spyOn(console, "error").mockImplementation(() => undefined);
    expect(checkShape("GET", "/api/me", { id: 1 })).toMatch(/неожиданном формате/);
    expect(log).toHaveBeenCalled();
    log.mockRestore();
  });

  it("путь без схемы и query-параметры", () => {
    expect(schemaFor("GET", "/api/meetings")).toBeUndefined();
    expect(schemaFor("GET", "/api/settings?x=1")).toBeDefined();
    expect(schemaFor("POST", "/api/settings")).toBeUndefined();
  });
});
