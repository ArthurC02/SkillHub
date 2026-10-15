import { afterEach, expect, test, vi } from "vitest";
import { apiFetch } from "../../core/api/client";
import { getSkillVersions } from "./skills.service";
import { SKILL, SKILL_VERSIONS } from "../../testing/fixtures/platform";

vi.mock("../../core/api/client", async (original) => ({
  ...(await original<typeof import("../../core/api/client")>()),
  apiFetch: vi.fn(),
}));
afterEach(() => vi.resetAllMocks());

const current = SKILL_VERSIONS.versions[0];
const older = SKILL_VERSIONS.versions[1];
const letterId = "abcdef01-2345-6789-abcd-0123456789ab";

test.each([
  ["empty", { versions: [] }],
  ["newest first", SKILL_VERSIONS],
  ["first version", { versions: [{ ...current, version_number: 1 }] }],
  ["largest exact version", { versions: [{ ...current, version_number: 9007199254740991 }] }],
  ["uppercase UUID", { versions: [{ ...current, version_id: letterId.toUpperCase() }] }],
  ["timezone offset", { versions: [{ ...current, created_at: "2026-08-17T08:00:00+08:00" }] }],
])("a complete %s version response retains the immutable snapshots", async (_name, response) => {
  vi.mocked(apiFetch).mockResolvedValue(response);
  await expect(getSkillVersions(SKILL)).resolves.toEqual(response);
  expect(apiFetch).toHaveBeenCalledExactlyOnceWith(`/skills/${SKILL}/versions`);
});

test.each([
  ["null body", null],
  ["missing list", {}],
  ["null list", { versions: null }],
  ["object list", { versions: { length: 1 } }],
  ["null item", { versions: [null] }],
  ["primitive item", { versions: ["version"] }],
  ["absent identity", { versions: [{ ...current, version_id: undefined }] }],
  ["invalid UUID", { versions: [{ ...current, version_id: "v1" }] }],
  ["short UUID", { versions: [{ ...current, version_id: current.version_id.slice(0, -1) }] }],
  ["long UUID", { versions: [{ ...current, version_id: `${current.version_id}0` }] }],
  ["text version", { versions: [{ ...current, version_number: "2" }] }],
  ["zero version", { versions: [{ ...current, version_number: 0 }] }],
  ["negative version", { versions: [{ ...current, version_number: -1 }] }],
  ["fractional version", { versions: [{ ...current, version_number: 1.5 }] }],
  ["inexact version", { versions: [{ ...current, version_number: 9007199254740992 }] }],
  ["absent hash", { versions: [{ ...current, content_hash: undefined }] }],
  ["blank hash", { versions: [{ ...current, content_hash: " " }] }],
  ["non-text hash", { versions: [{ ...current, content_hash: 123 }] }],
  ["absent date", { versions: [{ ...current, created_at: undefined }] }],
  ["invalid date", { versions: [{ ...current, created_at: "yesterday" }] }],
  ["partial list", { versions: [current, null] }],
  ["oldest first", { versions: [older, current] }],
  ["duplicate identity", { versions: [current, { ...older, version_id: current.version_id }] }],
  [
    "duplicate uppercase identity",
    {
      versions: [
        { ...current, version_id: letterId },
        { ...older, version_id: letterId.toUpperCase() },
      ],
    },
  ],
  [
    "duplicate number",
    { versions: [current, { ...older, version_number: current.version_number }] },
  ],
])("a %s response cannot confirm version history", async (_name, response) => {
  vi.mocked(apiFetch).mockResolvedValue(response);
  await expect(getSkillVersions(SKILL)).rejects.toThrow("Invalid skill version list response");
});
