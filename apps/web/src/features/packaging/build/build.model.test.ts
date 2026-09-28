import { expect, test } from "vitest";
import type { CreatedDownloadArtifact, PackagingTarget } from "../packaging.service";
import { matchingBuiltArtifact, resolveTarget } from "./build.model";

const TARGETS: PackagingTarget[] = [
  { id: "standard", label: "標準套件" } as unknown as PackagingTarget,
  { id: "claude-code", label: "Claude Code" } as unknown as PackagingTarget,
];

test("resolveTarget keeps the user's explicit choice over the default", () => {
  expect(resolveTarget("claude-code", TARGETS)).toBe("claude-code");
});

test("resolveTarget falls back to the first available target when nothing is chosen", () => {
  expect(resolveTarget("", TARGETS)).toBe("standard");
});

test("resolveTarget is empty when nothing is chosen and no targets loaded yet", () => {
  expect(resolveTarget("", undefined)).toBe("");
});

const ARTIFACT = {
  artifact_id: "a-1",
  skill_id: "skill-1",
  file_name: "skill.zip",
  duplicate: false,
} as unknown as CreatedDownloadArtifact;

test("matchingBuiltArtifact returns the build result when it matches this skill and version", () => {
  const build = { data: ARTIFACT, variables: { versionId: "v-1" } };

  expect(matchingBuiltArtifact(build, "skill-1", "v-1")).toBe(ARTIFACT);
});

test("matchingBuiltArtifact hides a stale result from a different skill", () => {
  const build = { data: ARTIFACT, variables: { versionId: "v-1" } };

  expect(matchingBuiltArtifact(build, "skill-2", "v-1")).toBeNull();
});

test("matchingBuiltArtifact hides a stale result after the version was switched", () => {
  const build = { data: ARTIFACT, variables: { versionId: "v-1" } };

  expect(matchingBuiltArtifact(build, "skill-1", "v-2")).toBeNull();
});
