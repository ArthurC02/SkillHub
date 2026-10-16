import { afterEach, expect, test, vi } from "vitest";
import { IMPORT_LIMITS } from "../../testing/fixtures/platform";
import { getSkillImportLimits } from "./import.service";

afterEach(() => vi.unstubAllGlobals());

const numericFields = [
  "max_zip_bytes",
  "max_unpacked_bytes",
  "max_files",
  "max_file_bytes",
  "max_path_depth",
] as const;

function reply(body: unknown) {
  const fetch = vi.fn((_input: RequestInfo | URL, _init?: RequestInit) =>
    Promise.resolve(new Response(JSON.stringify(body))),
  );
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

test.each<[string, unknown]>([
  ["current limits", IMPORT_LIMITS],
  ["no allowed hosts", { ...IMPORT_LIMITS, allowed_hosts: [] }],
  ["empty note", { ...IMPORT_LIMITS, note: "" }],
  ["minimum skill count", { ...IMPORT_LIMITS, max_skills_per_import: 1 }],
  [
    "largest safe skill count",
    { ...IMPORT_LIMITS, max_skills_per_import: Number.MAX_SAFE_INTEGER },
  ],
  ["additional field", { ...IMPORT_LIMITS, future: true }],
  ...numericFields.flatMap((field): [string, unknown][] => [
    [`zero ${field}`, { ...IMPORT_LIMITS, [field]: 0 }],
    [`largest safe ${field}`, { ...IMPORT_LIMITS, [field]: Number.MAX_SAFE_INTEGER }],
  ]),
])("readable import rules with %s are preserved", async (_label, body) => {
  const fetch = reply(body);
  await expect(getSkillImportLimits()).resolves.toEqual(body);
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0]?.[0]).toBe("/skills/import/limits");
  expect(fetch.mock.calls[0]?.[1]?.method ?? "GET").toBe("GET");
});

test.each<[string, unknown]>([
  ["null body", null],
  ["array body", []],
  ["primitive body", true],
  ["missing hosts", { ...IMPORT_LIMITS, allowed_hosts: undefined }],
  ["unreadable hosts", { ...IMPORT_LIMITS, allowed_hosts: [null] }],
  ["missing note", { ...IMPORT_LIMITS, note: undefined }],
  ["object note", { ...IMPORT_LIMITS, note: {} }],
  ...([...numericFields, "max_skills_per_import"] as const).flatMap(
    (field): [string, unknown][] => [
      [`missing ${field}`, { ...IMPORT_LIMITS, [field]: undefined }],
      [`null ${field}`, { ...IMPORT_LIMITS, [field]: null }],
      [`string ${field}`, { ...IMPORT_LIMITS, [field]: "10" }],
      [`fractional ${field}`, { ...IMPORT_LIMITS, [field]: 1.5 }],
      [`unsafe ${field}`, { ...IMPORT_LIMITS, [field]: Number.MAX_SAFE_INTEGER + 1 }],
    ],
  ),
  ["zero skill count", { ...IMPORT_LIMITS, max_skills_per_import: 0 }],
])("unreadable import rules with %s are rejected", async (_label, body) => {
  reply(body);
  await expect(getSkillImportLimits()).rejects.toThrow("Invalid import limits");
});
