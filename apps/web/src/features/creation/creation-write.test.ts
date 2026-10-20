import { afterEach, expect, test, vi } from "vitest";
import { ApiError } from "../../core/api/client";
import { actOnCreationSession, CreationWriteResultUnknown } from "./creation.service";

const action = {
  command_id: "33333333-3333-4333-8333-333333333333",
  expected_revision: 7,
  kind: "message" as const,
  message: "請保留原請求",
};

afterEach(() => vi.unstubAllGlobals());

test.each([
  [
    "unavailable",
    () =>
      new Response(JSON.stringify({ error: "創作服務暫時無法完成這個動作，進度已保留。" }), {
        status: 503,
      }),
  ],
  ["unreadable JSON", () => new Response("{broken", { status: 200 })],
  ["missing body", () => new Response(null, { status: 204 })],
] as const)(
  "a creation write with %s response stays uncertain without automatically resending",
  async (_, reply) => {
    const fetcher = vi.fn(() => Promise.resolve(reply()));
    vi.stubGlobal("fetch", fetcher);
    const outcome = await actOnCreationSession("s1", action).catch((error: unknown) => error);
    expect(outcome).toBeInstanceOf(CreationWriteResultUnknown);
    expect(outcome).toMatchObject({ sessionId: "s1" });
    expect(fetcher).toHaveBeenCalledOnce();
  },
);

test.each([
  [401, "not authenticated"],
  [404, "找不到可讀取的創作或參考內容。"],
  [409, "創作進度已改變，請重新讀取後確認。"],
  [422, "請確認目前步驟需要的輸入與草稿。"],
])(
  "a definite creation rejection with %i remains its original API error",
  async (status, message) => {
    const fetcher = vi.fn(() =>
      Promise.resolve(new Response(JSON.stringify({ error: message }), { status: Number(status) })),
    );
    vi.stubGlobal("fetch", fetcher);
    const outcome = await actOnCreationSession("s1", action).catch((error: unknown) => error);
    expect(outcome).toBeInstanceOf(ApiError);
    expect(outcome).toMatchObject({ status, message });
    expect(fetcher).toHaveBeenCalledOnce();
  },
);
