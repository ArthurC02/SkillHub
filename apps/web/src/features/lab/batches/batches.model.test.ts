import { expect, test } from "vitest";
import { BATCH, BATCH_REPORT, evaluationBatch } from "../../../testing/fixtures/platform";
import type { BatchReport, TrialOutcome } from "./batches.model";
import { haltSentence, reportStillMoving } from "./batches.model";

function reportWith(status: BatchReport["batch"]["status"], outcome: TrialOutcome): BatchReport {
  return {
    ...BATCH_REPORT,
    batch: evaluationBatch(BATCH, { status }),
    test_cases: [{ ...BATCH_REPORT.test_cases[0], trials: [{ trial: 1, run_id: BATCH, outcome }] }],
    skill_withheld: undefined,
  };
}

test.each([
  ["launching", "met", true],
  ["launched", "running", true],
  ["launched", "awaiting_answer", true],
  ["launched", "pending", true],
  ["launched", "met", false],
  ["launched", "undetermined", false],
  ["paused", "pending", false],
  ["stopped", "running", false],
] as const)("a %s batch with a %s trial keeps refreshing: %s", (status, outcome, moving) => {
  expect(reportStillMoving(reportWith(status, outcome))).toBe(moving);
});

test("an unsettled trial in the skill-withheld arm keeps a launched batch refreshing", () => {
  const report: BatchReport = {
    ...reportWith("launched", "met"),
    skill_withheld: {
      ...BATCH_REPORT.skill_withheld!,
      test_cases: [
        {
          ...BATCH_REPORT.skill_withheld!.test_cases[0],
          trials: [{ trial: 1, outcome: "running" }],
        },
      ],
    },
  };
  expect(reportStillMoving(report)).toBe(true);
});

test.each([
  ["launching", undefined, undefined],
  ["launched", undefined, undefined],
  ["paused", "quota_daily", "今天的試跑次數用完了，批次暫停了。次數恢復後按「繼續」。"],
  ["stopped", "paused_too_long", "暫停超過 30 天，批次已停止。"],
  ["stopped", "scan_blocked", "有一次試跑被拒絕，批次停止了（原因代碼：scan_blocked）。"],
  ["paused", undefined, "有一次試跑被拒絕，批次暫停了（原因代碼：未提供）。"],
] as const)("a %s batch with reason %s explains itself as %s", (status, reason, sentence) => {
  expect(haltSentence(evaluationBatch(BATCH, { status, stop_reason: reason }))).toBe(sentence);
});
