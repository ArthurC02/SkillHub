import { afterEach, expect, test, vi } from "vitest";
import { apiFetch, ApiError, SessionNotReadyError } from "../../core/api/client";
import { continueRun, isContinuationResultUnknown } from "./runs.service";

vi.mock("../../core/api/client", async (original) => ({
  ...(await original<typeof import("../../core/api/client")>()),
  apiFetch: vi.fn(),
}));
afterEach(() => vi.resetAllMocks());

const ORIGINAL = "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20";
const NEXT = "5c2f8e10-0b7a-4d63-9a41-7e3d2b1c9f44";

test.each([NEXT, NEXT.toUpperCase()])(
  "a continuation with a new UUID %s retains its confirmed destination",
  async (runId) => {
    vi.mocked(apiFetch).mockResolvedValue({ run_id: runId });
    await expect(continueRun(ORIGINAL, ["Sheet2", "最新"])).resolves.toEqual({ run_id: runId });
    expect(apiFetch).toHaveBeenCalledExactlyOnceWith(`/runs/${ORIGINAL}/continuations`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: '{"answers":["Sheet2","最新"]}',
    });
  },
);

test.each([
  ["null", null],
  ["primitive", "accepted"],
  ["empty", {}],
  ["array", []],
  ["numeric identity", { run_id: 123 }],
  ["blank identity", { run_id: " " }],
  ["short identity", { run_id: NEXT.slice(0, -1) }],
  ["long identity", { run_id: `${NEXT}0` }],
  ["invalid hexadecimal identity", { run_id: `g${NEXT.slice(1)}` }],
  ["same Run", { run_id: ORIGINAL }],
  ["same uppercase Run", { run_id: ORIGINAL.toUpperCase() }],
])("a %s acknowledgement cannot confirm a new Run", async (_name, receipt) => {
  vi.mocked(apiFetch).mockResolvedValue(receipt);
  await expect(continueRun(ORIGINAL, ["Sheet2"])).rejects.toThrow(
    "Invalid continuation Run response",
  );
});

test.each<[string, Error, boolean]>([
  ["lost response", new TypeError("Failed to fetch"), true],
  ["malformed JSON", new SyntaxError("Unexpected end of JSON input"), true],
  ["invalid acknowledgement", new Error("Invalid continuation Run response"), true],
  ["last client status", new ApiError(499, "client error"), false],
  ["first server status", new ApiError(500, "internal error"), true],
  ["server unavailable", new ApiError(503, "service unavailable"), true],
  ["login required", new ApiError(401, "not authenticated"), false],
  ["already answered", new ApiError(409, "already answered"), false],
  ["round limit", new ApiError(422, "refused"), false],
  ["request not sent", new SessionNotReadyError(), false],
])("%s preserves the distinction between refusal and unknown outcome", (_name, error, unknown) => {
  expect(isContinuationResultUnknown(error)).toBe(unknown);
});
