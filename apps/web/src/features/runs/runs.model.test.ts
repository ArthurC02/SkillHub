import { describe, expect, test } from "vitest";
import { runActivityGroup, runAttentionAction } from "./runs.model";

describe("runActivityGroup", () => {
  test.each([
    ["running", "not_met", "in_flight"],
    ["failed", "not_evaluated", "needs_attention"],
    ["timed_out", "not_evaluated", "needs_attention"],
    ["succeeded", "not_met", "needs_attention"],
    ["succeeded", "partially_met", "needs_attention"],
    ["succeeded", "undetermined", "needs_attention"],
    ["succeeded", "evaluation_failed", "needs_attention"],
    ["cancelled", "not_evaluated", "recent"],
    ["cancelled", "not_met", "needs_attention"],
    ["unknown", "not_evaluated", "recent"],
    ["succeeded", "met", "recent"],
  ])("groups %s with %s as %s", (status, verdict, expected) => {
    expect(runActivityGroup(status, verdict)).toBe(expected);
  });
});

describe("runAttentionAction", () => {
  test.each([
    ["failed", "not_evaluated", "查看原因"],
    ["timed_out", "not_evaluated", "查看原因"],
    ["succeeded", "evaluation_failed", "查看評估狀態"],
    ["succeeded", "not_met", "檢視證據"],
  ])("labels %s with %s as %s", (status, verdict, expected) => {
    expect(runAttentionAction(status, verdict)).toBe(expected);
  });
});
