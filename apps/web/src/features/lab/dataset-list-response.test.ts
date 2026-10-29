import { afterEach, expect, test, vi } from "vitest";
import { getTestCaseDatasets } from "./testcases.service";

afterEach(() => vi.unstubAllGlobals());

const row = {
  dataset_id: "abcdefab-1111-4111-8111-111111111111",
  file_name: "rows.csv",
  content_type: "text/csv",
  content_hash: "a".repeat(64),
  expires_at: "2026-11-15T00:00:00Z",
  size_bytes: 1,
};
const valid: [string, unknown][] = [
  ["empty", { datasets: [], total_bytes: 0 }],
  ["zero-sized file", { datasets: [{ ...row, size_bytes: 0 }], total_bytes: 0 }],
  [
    "multiple files",
    {
      datasets: [
        row,
        { ...row, dataset_id: "22222222-2222-4222-8222-222222222222", size_bytes: 2 },
      ],
      total_bytes: 3,
    },
  ],
  [
    "largest safe total",
    { datasets: [{ ...row, size_bytes: 9007199254740991 }], total_bytes: 9007199254740991 },
  ],
  [
    "uppercase UUID",
    { datasets: [{ ...row, dataset_id: row.dataset_id.toUpperCase() }], total_bytes: 1 },
  ],
  [
    "nil UUID",
    { datasets: [{ ...row, dataset_id: "00000000-0000-0000-0000-000000000000" }], total_bytes: 1 },
  ],
  [
    "open display strings",
    { datasets: [{ ...row, file_name: "", content_type: "", content_hash: "" }], total_bytes: 1 },
  ],
  ["extra metadata", { datasets: [{ ...row, extra: "kept" }], total_bytes: 1, extra: "kept" }],
];

test.each(valid)("dataset list with %s preserves the complete response", async (_name, body) => {
  const fetch = vi.fn(() => Promise.resolve(new Response(JSON.stringify(body), { status: 200 })));
  vi.stubGlobal("fetch", fetch);
  await expect(getTestCaseDatasets("test-case")).resolves.toEqual(body);
  expect(fetch).toHaveBeenCalledExactlyOnceWith("/test-cases/test-case/datasets", {
    credentials: "include",
    headers: { Accept: "application/json" },
  });
});

const invalid: [string, unknown][] = [
  ["null body", null],
  ["primitive body", false],
  ["array body", []],
  ["missing files", { total_bytes: 0 }],
  ...[null, "", {}].map((datasets): [string, unknown] => [
    "non-array files " + JSON.stringify(datasets),
    { datasets, total_bytes: 0 },
  ]),
  ["missing total", { datasets: [] }],
  ...[null, "0", -1, 0.5, 9007199254740992].map((total_bytes): [string, unknown] => [
    "invalid total " + JSON.stringify(total_bytes),
    { datasets: [], total_bytes },
  ]),
  ...[null, [], false].map((value): [string, unknown] => [
    "invalid row " + JSON.stringify(value),
    { datasets: [value], total_bytes: 0 },
  ]),
  ...Object.keys(row).map((field): [string, unknown] => [
    "missing " + field,
    { datasets: [{ ...row, [field]: undefined }], total_bytes: 1 },
  ]),
  ...Object.entries({
    dataset_id: "wrong",
    file_name: 3,
    content_type: null,
    content_hash: false,
    expires_at: "not-a-time",
  }).map(([field, value]): [string, unknown] => [
    "invalid " + field,
    { datasets: [{ ...row, [field]: value }], total_bytes: 1 },
  ]),
  ...[-1, 0.5, 9007199254740992, "1", null].map((size_bytes): [string, unknown] => [
    "invalid size " + JSON.stringify(size_bytes),
    { datasets: [{ ...row, size_bytes }], total_bytes: 1 },
  ]),
  ["partly unreadable files", { datasets: [row, {}], total_bytes: 1 }],
  ["empty files with nonzero total", { datasets: [], total_bytes: 1 }],
  ["underreported total", { datasets: [row], total_bytes: 0 }],
  ["overreported total", { datasets: [row], total_bytes: 2 }],
];

test.each(invalid)("dataset list with %s remains unreadable", async (_name, body) => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => Promise.resolve(new Response(JSON.stringify(body), { status: 200 }))),
  );
  await expect(getTestCaseDatasets("test-case")).rejects.toThrow("Invalid dataset list response");
});
