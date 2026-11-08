import { expect, test } from "vitest";
import { RunStatus } from "@skillhub/api-client-ts";
import { awaitsEvaluation, evaluationPanelState } from "./evaluation.model";

const AWAITS_EVALUATION = new Set(["succeeded", "failed"]);

test.each(Object.values(RunStatus))(
  "a %s run is polled for an evaluation exactly when the panel says one is coming",
  (status) => {
    const expected = AWAITS_EVALUATION.has(status);
    const panel = evaluationPanelState({
      runStatus: status,
      revision: undefined,
      error: undefined,
      errorUpdateCount: 0,
      dataStatus: undefined,
      pending: false,
    });

    expect(awaitsEvaluation(status)).toBe(expected);
    expect(panel.awaiting).toBe(expected);
  },
);

test("a run whose status is not known yet is not polled for an evaluation", () => {
  expect(awaitsEvaluation(undefined)).toBe(false);
});
