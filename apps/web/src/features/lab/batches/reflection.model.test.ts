import { expect, test } from "vitest";
import { heldOutTestCaseIds, isBatchReflection } from "./reflection.model";

const round = {
  reflection_id: "r",
  batch_id: "B1",
  dev_test_case_ids: ["A"],
  outcome: "kept",
  runs_read: 1,
  runs_not_read: 0,
  created_at: "2026-11-07T01:00:00Z",
  suggestions: [],
};

test.each([
  ["the asked batch, any case", round, true],
  ["another batch", { ...round, batch_id: "B2" }, false],
  ["an outcome the contract does not name", { ...round, outcome: "fine" }, false],
  ["a negative count", { ...round, runs_not_read: -1 }, false],
])("a round for %s is accepted: %s", (_name, body, accepted) => {
  expect(isBatchReflection(body, "b1")).toBe(accepted);
});

test("the held-out cases are the batch's cases the round did not read, in batch order", () => {
  expect(
    heldOutTestCaseIds(["a", "B", "c"], { ...round, dev_test_case_ids: ["b"] } as never),
  ).toEqual(["a", "c"]);
});
