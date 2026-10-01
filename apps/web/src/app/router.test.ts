import { describe, expect, test } from "vitest";
import {
  legacyDatasetDestination,
  legacyLibraryDestination,
  legacyRunDestination,
  validatePublishingWorkspaceSearch,
} from "./router";

describe("legacy Library links", () => {
  test.each([
    ["the list", "", { to: "/library", hash: "" }],
    ["the create anchor", "create", { to: "/library", hash: "create" }],
  ])("sends %s to the canonical platform space", (_name, hash, expected) => {
    expect(legacyLibraryDestination(hash)).toEqual(expected);
  });
});

describe("publishing workspace search", () => {
  test.each([
    [
      "valid artifact",
      { artifact: "44444444-4444-4444-4444-444444444444" },
      {
        artifact: "44444444-4444-4444-4444-444444444444",
        publication: undefined,
        bundleVersion: undefined,
      },
    ],
    [
      "invalid artifact",
      { artifact: "artifact-1" },
      { artifact: undefined, publication: undefined, bundleVersion: undefined },
    ],
    [
      "valid publication",
      { publication: "skillhub/pdf-summariser" },
      {
        artifact: undefined,
        publication: "skillhub/pdf-summariser",
        bundleVersion: undefined,
      },
    ],
    [
      "invalid publication",
      { publication: "skillhub/pdf/summariser" },
      { artifact: undefined, publication: undefined, bundleVersion: undefined },
    ],
    [
      "valid Bundle member Version",
      { bundleVersion: "22222222-2222-2222-2222-111111111111" },
      {
        artifact: undefined,
        publication: undefined,
        bundleVersion: "22222222-2222-2222-2222-111111111111",
      },
    ],
    [
      "invalid Bundle member Version",
      { bundleVersion: "version-1" },
      { artifact: undefined, publication: undefined, bundleVersion: undefined },
    ],
    [
      "two valid targets remain visible as a conflict",
      {
        artifact: "44444444-4444-4444-4444-444444444444",
        publication: "skillhub/pdf-summariser",
        bundleVersion: "22222222-2222-2222-2222-111111111111",
      },
      {
        artifact: "44444444-4444-4444-4444-444444444444",
        publication: "skillhub/pdf-summariser",
        bundleVersion: "22222222-2222-2222-2222-111111111111",
      },
    ],
  ])("keeps %s according to its identity shape", (_name, search, expected) => {
    expect(validatePublishingWorkspaceSearch(search)).toEqual(expected);
  });
});

describe("legacy 試跑紀錄 preflight links", () => {
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
      "a 測試題 without its 小工具",
      { version: "version-1", test_case: "test-case-1" },
      {
        to: "/lab/test-cases/$testCaseId",
        params: { testCaseId: "test-case-1" },
        search: { version: "version-1" },
      },
    ],
    [
      "no 測試題",
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
      "a 測試題 and version",
      { test_case: "test-case-1", version: "version-1" },
      {
        to: "/lab/test-cases/$testCaseId/datasets",
        params: { testCaseId: "test-case-1" },
        search: { version: "version-1" },
      },
    ],
    ["no 測試題", {}, { to: "/lab/test-cases", search: {} }],
  ])("sends %s to the nearest durable context", (_name, search, expected) => {
    expect(legacyDatasetDestination(search)).toEqual(expected);
  });
});
