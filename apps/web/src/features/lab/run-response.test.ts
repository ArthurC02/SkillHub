import { afterEach, expect, test, vi } from "vitest";
import { apiFetch } from "../../core/api/client";
import { startRun } from "./lab.service";

vi.mock("../../core/api/client", async (original) => ({
  ...(await original<typeof import("../../core/api/client")>()),
  apiFetch: vi.fn(),
}));
afterEach(() => vi.resetAllMocks());

const receipt = { run_id: "run-1", status: "queued", provider: "unassigned" };

test("a complete Run receipt retains the permanent result identifier", async () => {
  vi.mocked(apiFetch).mockResolvedValue(receipt);
  await expect(startRun("skill", "version", "test-case", "hash")).resolves.toEqual({
    run_id: "run-1",
    status: "queued",
    provider: "unassigned",
  });
});

test.each([
  ["null", null],
  ["empty", {}],
  ["blank identifier", { ...receipt, run_id: " " }],
  ["missing provider", { ...receipt, provider: undefined }],
  ["missing status", { ...receipt, status: undefined }],
  ["numeric identifier", { ...receipt, run_id: 1 }],
])("a %s response cannot acknowledge a Run", async (_name, response) => {
  vi.mocked(apiFetch).mockResolvedValue(response);
  await expect(startRun("skill", "version", "test-case", "hash")).rejects.toThrow(
    "Invalid run response",
  );
});
