import { expect, test, vi, afterEach } from "vitest";
import { apiFetch } from "../../core/api/client";
import { uploadDataset } from "./lab.service";

vi.mock("../../core/api/client", async (original) => ({
  ...(await original<typeof import("../../core/api/client")>()),
  apiFetch: vi.fn(),
}));
afterEach(() => vi.resetAllMocks());

const receipt = {
  dataset_id: "11111111-1111-4111-8111-111111111111",
  file_name: "rows.csv",
  content_type: "text/csv",
  content_hash: "a".repeat(64),
  expires_at: "2026-11-15T00:00:00Z",
  size_bytes: 0,
};

test.each([0, 9007199254740991])("a complete upload receipt accepts size %i", async (size) => {
  const response = { ...receipt, size_bytes: size };
  vi.mocked(apiFetch).mockResolvedValue(response);
  await expect(uploadDataset("test-case", new File([], "rows.csv"))).resolves.toEqual(response);
});

test.each([
  ["null", null],
  ["empty", {}],
  ["missing expiry", { ...receipt, expires_at: undefined }],
  ["blank filename", { ...receipt, file_name: " " }],
  ["negative size", { ...receipt, size_bytes: -1 }],
  ["fractional size", { ...receipt, size_bytes: 0.5 }],
  ["unsafe size", { ...receipt, size_bytes: 9007199254740992 }],
  ["string size", { ...receipt, size_bytes: "0" }],
])("a %s response cannot acknowledge an upload", async (_name, response) => {
  vi.mocked(apiFetch).mockResolvedValue(response);
  await expect(uploadDataset("test-case", new File([], "rows.csv"))).rejects.toThrow(
    "Invalid dataset response",
  );
});
