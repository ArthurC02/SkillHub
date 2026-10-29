import { afterEach, expect, test, vi } from "vitest";
import { ADMIN_SKILLS } from "../../../testing/fixtures/platform";
import { getGovernance } from "../admin.service";

afterEach(() => vi.unstubAllGlobals());
const SKILL = ADMIN_SKILLS.skills[0];

test.each<[string, unknown]>([
  ["empty matches", { skills: [] }],
  ["active skill", ADMIN_SKILLS],
  [
    "owner-deleted held skill",
    {
      skills: [
        {
          ...SKILL,
          deleted_at: "2026-10-20T00:00:00Z",
          takedown_at: "2026-10-21T08:00:00+08:00",
          takedown_reason: "hold",
          access_restriction: "policy-review",
        },
      ],
    },
  ],
  [
    "open strings and metadata",
    {
      extra: true,
      skills: [
        {
          ...SKILL,
          redistribution: "future-policy",
          name: "",
          access_restriction: "other-code",
          extra: true,
        },
      ],
    },
  ],
  ["uppercase UUID", { skills: [{ ...SKILL, skill_id: "ABCDEFAB-CDEF-ABCD-EFAB-CDEFABCDEFAB" }] }],
])("governance with %s preserves the complete response", async (_name, body) => {
  const fetch = vi.fn(() => Promise.resolve(new Response(JSON.stringify(body), { status: 200 })));
  vi.stubGlobal("fetch", fetch);
  await expect(getGovernance("name & query")).resolves.toEqual(body);
  expect(fetch).toHaveBeenCalledExactlyOnceWith("/admin/skills?q=name%20%26%20query", {
    credentials: "include",
    headers: { Accept: "application/json" },
  });
});

test.each<[string, unknown]>([
  ["null body", null],
  ["primitive body", false],
  ["array body", []],
  ["missing matches", {}],
  ["null matches", { skills: null }],
  ["string matches", { skills: "" }],
  ["object matches", { skills: {} }],
  ["null skill", { skills: [null] }],
  ["array skill", { skills: [[]] }],
  ["primitive skill", { skills: [false] }],
  ...Object.keys(SKILL).map((key): [string, unknown] => [
    "missing " + key,
    { skills: [{ ...SKILL, [key]: undefined }] },
  ]),
  ...(
    [
      ["skill_id", "not-a-uuid"],
      ["workspace_id", "not-a-uuid"],
      ["name", 3],
      ["redistribution", null],
      ["access_restriction", false],
      ["takedown_reason", {}],
      ["takedown_at", 3],
      ["deleted_at", 3],
      ["takedown_at", "not-a-time"],
      ["deleted_at", "not-a-time"],
    ] as [string, unknown][]
  ).map(([key, value]): [string, unknown] => [
    "invalid " + key + " " + String(value),
    { skills: [{ ...SKILL, [key]: value }] },
  ]),
  ["partly unreadable matches", { skills: [SKILL, { ...SKILL, takedown_at: undefined }] }],
])("governance with %s remains unreadable", async (_name, body) => {
  vi.stubGlobal("fetch", () =>
    Promise.resolve(new Response(JSON.stringify(body), { status: 200 })),
  );
  await expect(getGovernance("query")).rejects.toThrow("Invalid governance response");
});
