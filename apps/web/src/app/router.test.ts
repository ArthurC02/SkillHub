import { describe, expect, test } from "vitest";
import {
  legacyDatasetDestination,
  legacyRunDestination,
  validatePublishingWorkspaceSearch,
} from "./router";

describe("publishing workspace search", () => {
  test.each([
    [
      "valid artifact",
      { artifact: "44444444-4444-4444-4444-444444444444" },
      {
        artifact: "44444444-4444-4444-4444-444444444444",
        publication: undefined,
      },
    ],
    [
      "invalid artifact",
      { artifact: "artifact-1" },
      { artifact: undefined, publication: undefined },
    ],
    [
      "valid publication",
      { publication: "skillhub/pdf-summariser" },
      { artifact: undefined, publication: "skillhub/pdf-summariser" },
    ],
    [
      "invalid publication",
      { publication: "skillhub/pdf/summariser" },
      { artifact: undefined, publication: undefined },
    ],
    [
      "two valid targets remain visible as a conflict",
      {
        artifact: "44444444-4444-4444-4444-444444444444",
        publication: "skillhub/pdf-summariser",
      },
      {
        artifact: "44444444-4444-4444-4444-444444444444",
        publication: "skillhub/pdf-summariser",
      },
    ],
  ])("keeps %s according to its identity shape", (_name, search, expected) => {
    expect(validatePublishingWorkspaceSearch(search)).toEqual(expected);
  });
});

describe("legacy Run preflight links", () => {
  test.each([
    [
      "complete object context",
      { skill: "skill-1", version: "version-1", test_case: "test-case-1" },
      {
        to: "/skills/$skillId/test-cases/$testCaseId/runs/new",
        params: { skillId: "skill-1", testCaseId: "test-case-1" },
        search: { version: "version-1" },
      },
    ],
    [
      "a Test Case without its Skill",
      { version: "version-1", test_case: "test-case-1" },
      {
        to: "/lab/test-cases/$testCaseId",
        params: { testCaseId: "test-case-1" },
        search: { version: "version-1" },
      },
    ],
    [
      "no Test Case",
      { skill: "skill-1", version: "version-1" },
      {
        to: "/lab/test-cases",
        search: { skill: "skill-1", version: "version-1" },
      },
    ],
  ])("sends %s to the nearest durable context", (_name, search, expected) => {
    expect(legacyRunDestination(search)).toEqual(expected);
  });
});

describe("legacy Dataset links", () => {
  test.each([
    [
      "a Test Case and version",
      { test_case: "test-case-1", version: "version-1" },
      {
        to: "/lab/test-cases/$testCaseId/datasets",
        params: { testCaseId: "test-case-1" },
        search: { version: "version-1" },
      },
    ],
    ["no Test Case", {}, { to: "/lab/test-cases", search: {} }],
  ])("sends %s to the nearest durable context", (_name, search, expected) => {
    expect(legacyDatasetDestination(search)).toEqual(expected);
  });
});
