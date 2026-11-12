import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, api, unwrap } from "./api";

describe("API client", () => {
  beforeEach(() => vi.stubGlobal("crypto", { randomUUID: vi.fn(() => "request-id") }));

  it("adds user and JSON headers", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ value: 1 }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    await expect(api<{ value: number }>("intern", "/api/me")).resolves.toEqual({ value: 1 });
    const [, init] = fetchMock.mock.calls[0];
    expect(new Headers(init.headers)).toMatchObject(expect.any(Headers));
    expect(new Headers(init.headers).get("X-User")).toBe("intern");
    expect(new Headers(init.headers).get("Content-Type")).toBe("application/json");
  });

  it("reuses an idempotency key after a server error and clears it after success", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: { message: "later" } }), { status: 500 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ ok: true }), { status: 201 }));
    vi.stubGlobal("fetch", fetchMock);
    const init = { method: "POST", body: JSON.stringify({ kind: "access" }) };
    await expect(api("intern", "/api/requests", init)).rejects.toEqual(expect.objectContaining({ status: 500, message: "later" }));
    await api("intern", "/api/requests", init);
    expect(new Headers(fetchMock.mock.calls[0][1].headers).get("Idempotency-Key")).toBe("request-id");
    expect(new Headers(fetchMock.mock.calls[1][1].headers).get("Idempotency-Key")).toBe("request-id");
  });

  it("uses a safe error for invalid error JSON", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("not json", { status: 403 })));
    await expect(api("u", "/api/private")).rejects.toEqual(new ApiError(403, "Не удалось выполнить запрос"));
  });

  it("unwraps successful envelopes and rejects incomplete ones", () => {
    expect(unwrap({ ok: true, data: 7 })).toBe(7);
    expect(() => unwrap({ ok: false, error: { code: "bad", message: "Ошибка" } })).toThrow("Ошибка");
    expect(() => unwrap({ ok: true })).toThrow("Нет данных");
  });
});
