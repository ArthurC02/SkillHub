import { afterEach, expect, test, vi } from "vitest";
import { getTestCases } from "./testcases.service";

afterEach(() => vi.unstubAllGlobals());

const row = {
  test_case_id: "abcdefab-1111-4111-8111-111111111111",
  name: "Scenario",
  skill_name: "Tool",
  criteria_confirmed: 1,
  criteria_total: 2,
  has_rubric: true,
  updated_at: "2026-11-01T00:00:00Z",
};
const listed = (length: number) =>
  Array.from({ length }, (_, index) => ({
    ...row,
    test_case_id: `00000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
  }));
const valid: [string, unknown][] = [
  ["empty", { test_cases: [] }],
  ["display projection", { test_cases: [row] }],
  ["full visible page", { test_cases: listed(50) }],
  ["page with sentinel", { test_cases: listed(51) }],
  ["uppercase UUID", { test_cases: [{ ...row, test_case_id: row.test_case_id.toUpperCase() }] }],
  ["nil UUID", { test_cases: [{ ...row, test_case_id: "00000000-0000-0000-0000-000000000000" }] }],
  [
    "no criteria or rubric",
    { test_cases: [{ ...row, criteria_confirmed: 0, criteria_total: 0, has_rubric: false }] },
  ],
  ["all criteria confirmed", { test_cases: [{ ...row, criteria_confirmed: 2 }] }],
  [
    "largest safe counts",
    {
      test_cases: [
        { ...row, criteria_confirmed: 9007199254740991, criteria_total: 9007199254740991 },
      ],
    },
  ],
  ["empty display names", { test_cases: [{ ...row, name: "", skill_name: "" }] }],
  ["additional metadata", { test_cases: [{ ...row, extra: "kept" }], extra: "kept" }],
];

test.each(valid)("test-case list with %s preserves the response", async (_name, body) => {
  const fetch = vi.fn(() => Promise.resolve(new Response(JSON.stringify(body), { status: 200 })));
  vi.stubGlobal("fetch", fetch);
  await expect(getTestCases()).resolves.toEqual(body);
  expect(fetch).toHaveBeenCalledExactlyOnceWith("/test-cases?limit=51&offset=0", {
    credentials: "include",
    headers: { Accept: "application/json" },
  });
});

test("test-case list keeps the server filter and offset in the request", async () => {
  const fetch = vi.fn(() => Promise.resolve(new Response('{"test_cases":[]}', { status: 200 })));
  vi.stubGlobal("fetch", fetch);
  await expect(getTestCases("skill /&", 50)).resolves.toEqual({ test_cases: [] });
  expect(fetch).toHaveBeenCalledExactlyOnceWith(
    "/test-cases?limit=51&offset=50&skill_id=skill+%2F%26",
    { credentials: "include", headers: { Accept: "application/json" } },
  );
});

const invalid: [string, unknown][] = [
  ["null body", null],
  ["primitive body", false],
  ["array body", []],
  ["missing rows", {}],
  ...[null, "", {}].map((test_cases): [string, unknown] => [
    "non-array rows " + JSON.stringify(test_cases),
    { test_cases },
  ]),
  ...[null, [], false].map((value): [string, unknown] => [
    "unreadable row " + JSON.stringify(value),
    { test_cases: [value] },
  ]),
  ...Object.keys(row).map((field): [string, unknown] => [
    "missing " + field,
    { test_cases: [{ ...row, [field]: undefined }] },
  ]),
  ...Object.entries({
    test_case_id: "wrong",
    name: 3,
    skill_name: null,
    updated_at: "not-a-time",
    has_rubric: "false",
  }).map(([field, value]): [string, unknown] => [
    "invalid " + field,
    { test_cases: [{ ...row, [field]: value }] },
  ]),
  ...["criteria_confirmed", "criteria_total"].flatMap((field) =>
    [-1, 0.5, 9007199254740992, "1", null].map((value): [string, unknown] => [
      `invalid ${field} ${JSON.stringify(value)}`,
      { test_cases: [{ ...row, [field]: value }] },
    ]),
  ),
  ["more confirmed than total", { test_cases: [{ ...row, criteria_confirmed: 3 }] }],
  ["partly unreadable rows", { test_cases: [row, {}] }],
  ["unreadable sentinel", { test_cases: [...listed(50), {}] }],
  ["more rows than requested", { test_cases: listed(52) }],
];

test.each(invalid)("test-case list with %s remains unreadable", async (_name, body) => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => Promise.resolve(new Response(JSON.stringify(body), { status: 200 }))),
  );
  await expect(getTestCases()).rejects.toThrow("Invalid test case list response");
});
