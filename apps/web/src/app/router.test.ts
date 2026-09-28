import { describe, expect, test } from "vitest";
import { validatePublishingWorkspaceSearch } from "./router";

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
