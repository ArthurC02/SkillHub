import { expect, test } from "vitest";
import type { OwnSkills } from "../../core/api/types";
import { platformResponse } from "../../testing/fixtures/platform";
import { readOwnSkillPage } from "./own-skills.model";

const row = (platformResponse("/skills").body as OwnSkills).skills[0];
const page = { skills: [row], limit: 100, total: 1, truncated: false };
const otherId = "abcdef01-2345-4789-abcd-0123456789ab";
const bounded = Array.from({ length: 100 }, (_, index) => ({
  ...row,
  skill_id: `00000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
}));

for (const [name, value, offset] of [
  ["one readable skill", page, 0],
  [
    "a null measurement timestamp",
    { ...page, skills: [{ ...row, verification: { ...row.verification, scanned_at: null } }] },
    0,
  ],
  ["an empty workspace", { ...page, skills: [], total: 0 }, 0],
  ["the minimum page limit", { ...page, limit: 1 }, 0],
  ["the maximum page limit filled", { ...page, skills: bounded, total: 100 }, 0],
  ["an incomplete first page", { ...page, limit: 1, total: 2, truncated: true }, 0],
  ["the final page retains the workspace total", { ...page, total: 101 }, 100],
  [
    "a readable lineage",
    {
      ...page,
      skills: [
        { ...row, forked_from_skill_id: otherId, forked_from_version_id: otherId.toUpperCase() },
      ],
    },
    0,
  ],
  [
    "an absent measurement timestamp",
    {
      ...page,
      skills: [
        { ...row, verification: { value: "not_measured", label: "未測量", note: "尚未測量" } },
      ],
    },
    0,
  ],
  [
    "the highest precise total",
    { ...page, limit: 1, total: Number.MAX_SAFE_INTEGER, truncated: true },
    0,
  ],
] as const) {
  test(`a workspace skill page accepts ${name}`, () => {
    expect(readOwnSkillPage(value, offset)).toEqual(value);
  });
}

function withRow(patch: Record<string, unknown>) {
  return { ...page, skills: [{ ...row, ...patch }] };
}

for (const [name, value] of [
  ["a null response", null],
  ["an array response", []],
  ["a text response", "unavailable"],
  ["a numeric response", 0],
  ["missing rows", { ...page, skills: undefined }],
  ["null rows", { ...page, skills: null }],
  ["a rows object", { ...page, skills: {} }],
  ["a null row", { ...page, skills: [null] }],
  [
    "an unreadable later row",
    { ...page, skills: [row, { ...row, skill_id: otherId, risk: {} }], limit: 2, total: 2 },
  ],
  ["an invalid skill identifier", withRow({ skill_id: "invalid" })],
  ["a non-text name", withRow({ name: 1 })],
  ["a non-text summary", withRow({ summary: null })],
  ["an unknown redistribution", withRow({ redistribution: "unrecognized" })],
  ["an unreadable restriction", withRow({ access_restriction: {} })],
  ["an invalid source identifier", withRow({ forked_from_skill_id: 1 })],
  ["an invalid source version", withRow({ forked_from_version_id: "invalid" })],
  ["unreadable risk", withRow({ risk: {} })],
  ["missing verification", withRow({ verification: undefined })],
  ["null verification", withRow({ verification: null })],
  [
    "an unknown verification",
    withRow({ verification: { ...row.verification, value: "unrecognized" } }),
  ],
  [
    "a missing verification label",
    withRow({ verification: { ...row.verification, label: undefined } }),
  ],
  ["an unreadable verification note", withRow({ verification: { ...row.verification, note: 1 } })],
  [
    "an invalid measurement time",
    withRow({ verification: { ...row.verification, scanned_at: "not-a-time" } }),
  ],
  ["duplicate identities", { ...page, skills: [row, row], limit: 2, total: 2 }],
  [
    "duplicate identities with different case",
    {
      ...page,
      skills: [
        { ...row, skill_id: otherId },
        { ...row, skill_id: otherId.toUpperCase() },
      ],
      limit: 2,
      total: 2,
    },
  ],
  ["a missing limit", { ...page, limit: undefined }],
  ["a zero limit", { ...page, limit: 0 }],
  ["a fractional limit", { ...page, limit: 1.5 }],
  ["a limit above the contract maximum", { ...page, limit: 101 }],
  ["a textual limit", { ...page, limit: "100" }],
  [
    "more rows than the declared limit",
    { ...page, skills: [row, { ...row, skill_id: otherId }], limit: 1, total: 2 },
  ],
  ["a missing total", { ...page, total: undefined }],
  ["a negative total", { ...page, total: -1 }],
  ["a total smaller than its rows", { ...page, total: 0 }],
  ["a fractional total", { ...page, total: 1.5 }],
  ["an imprecise total", { ...page, total: Number.MAX_SAFE_INTEGER + 1 }],
  ["a textual total", { ...page, total: "1" }],
  ["missing coverage", { ...page, truncated: undefined }],
  ["null coverage", { ...page, truncated: null }],
  ["textual coverage", { ...page, truncated: "false" }],
  ["an empty page claiming more rows", { ...page, skills: [], total: 1, truncated: true }],
  ["coverage claiming more than its total", { ...page, truncated: true }],
  ["a complete first page missing a row", { ...page, total: 2 }],
] as const) {
  test(`a workspace skill page rejects ${name}`, () => {
    expect(() => readOwnSkillPage(value)).toThrow("Invalid workspace skill list response");
  });
}
