import { afterEach, describe, expect, it, vi } from "vitest";
import { subscribeChanges } from "@/shared/api";

const streamOf = (chunks: string[]) => new Response(new ReadableStream({
  start(c) { for (const x of chunks) c.enqueue(new TextEncoder().encode(x)); c.close(); },
}), { status: 200, headers: { "Content-Type": "text/event-stream" } });

describe("живые обновления", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("«changed» из потока — обновить экран; «ready» и ping — нет; X-User в заголовке", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(streamOf(["retry: 5000\nevent: ready\ndata: 1\n\n: ping\n\nevent: chan", "ged\ndata: 2\n\n"]))
      .mockResolvedValue(new Response("", { status: 404 }));
    vi.stubGlobal("fetch", fetchMock);
    const onChange = vi.fn();
    const stop = subscribeChanges("intern.sokolov", onChange, 10);
    await vi.waitFor(() => expect(onChange).toHaveBeenCalledTimes(1));
    expect(new Headers(fetchMock.mock.calls[0][1].headers).get("X-User")).toBe("intern.sokolov");
    await vi.waitFor(() => expect(fetchMock.mock.calls.length).toBeGreaterThan(1)); // поток закрылся — переподключение
    stop();
    const calls = fetchMock.mock.calls.length;
    await new Promise((r) => setTimeout(r, 40));
    expect(fetchMock.mock.calls.length).toBe(calls); // после отписки — тишина
  });
});
