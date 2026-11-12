import type {
  CandidatePairedVerdictVerdictEnum,
  EvaluationBatchStatusEnum,
  EvaluationBatchTrialOutcomeEnum,
} from "@skillhub/api-client-ts";
import {
  CreateEvaluationBatchBodyTestCaseIdsMaxItems as MAX_TEST_CASES,
  CreateEvaluationBatchBodyTrialsMaximum as MAX_TRIALS,
} from "@skillhub/api-client-ts/limits";
import { isCount, isOptionalText, isRecord } from "../../../core/api/wire";

export type BatchStatus = EvaluationBatchStatusEnum;

export type TrialOutcome = EvaluationBatchTrialOutcomeEnum;

export type EvaluationBatch = {
  id: string;
  skill_id: string;
  skill_version_id: string;
  trials: number;
  with_skill_withheld: boolean;
  status: BatchStatus;
  stop_reason?: string;
  created_at: string;
};

export type Interval = { low: number; high: number };

export type BatchTrial = {
  trial: number;
  run_id?: string;
  end_run_id?: string;
  outcome: TrialOutcome;
};

export type BatchTestCase = {
  test_case_id: string;
  trials: BatchTrial[];
  passed: number;
  judged: number;
  pass_at_k?: boolean;
  pass_all_k?: boolean;
  prompt_varied_across: boolean;
};

export type BatchArm = {
  test_cases: BatchTestCase[];
  passed: number;
  judged: number;
  pass_rate?: number;
  pass_rate_95?: Interval;
  test_cases_all_passed: number;
  test_cases_decided: number;
};

export type PairedDifference = {
  paired: number;
  unpaired: number;
  difference?: number;
  difference_95?: Interval;
  verdict: DifferenceVerdict;
  better: number;
  worse: number;
  tied: number;
};

export type BatchReport = BatchArm & {
  batch: EvaluationBatch;
  skill_withheld?: BatchArm;
  skill_effect?: PairedDifference;
};

export type BatchComparison = {
  reference: EvaluationBatch;
  candidate: EvaluationBatch;
  paired: number;
  unpaired: number;
  difference?: number;
  difference_95?: Interval;
  verdict: DifferenceVerdict;
  candidate_better: number;
  reference_better: number;
  tied: number;
};

export const STATUS_LABEL: Record<BatchStatus, string> = {
  launching: "試跑陸續開始中",
  launched: "每次試跑都已開始",
  paused: "已暫停",
  stopped: "已停止",
};

export const OUTCOME_LABEL: Record<TrialOutcome, string> = {
  pending: "尚未開始",
  running: "執行中",
  awaiting_answer: "等待回答",
  met: "符合",
  partially_met: "部分符合",
  not_met: "未符合",
  undetermined: "無法判斷",
  not_evaluated: "未評估",
};

const HALT_SENTENCE: Record<string, string> = {
  credit_balance:
    "點數不夠開下一次試跑，批次暫停了。加點後按「繼續」；已經跑過的試跑不會重跑，也不會再扣點。",
  quota_daily: "今天的試跑次數用完了，批次暫停了。次數恢復後按「繼續」。",
  quota_window: "這段期間的試跑次數用完了，批次暫停了。次數恢復後按「繼續」。",
  paused_too_long: "暫停超過 30 天，批次已停止。",
  permissions_unconfirmed:
    "批次進行中有測試題的試跑權限變了，批次已停止。重新確認權限後再開一個批次。",
  target_gone: "這個版本或其中一題測試題已被刪除，批次已停止。",
  creator_gone: "開這個批次的帳號已不存在，批次已停止。",
};

export function haltSentence(batch: EvaluationBatch): string | undefined {
  if (batch.status !== "paused" && batch.status !== "stopped") return undefined;
  const reason = batch.stop_reason ?? "";
  return (
    HALT_SENTENCE[reason] ??
    `有一次試跑被拒絕，批次${batch.status === "paused" ? "暫停" : "停止"}了（原因代碼：${reason || "未測量"}）。`
  );
}

const STATUSES = new Set<string>(Object.keys(STATUS_LABEL));
const OUTCOMES = new Set<string>(Object.keys(OUTCOME_LABEL));
const UNSETTLED_OUTCOMES = new Set<TrialOutcome>(["pending", "running", "awaiting_answer"]);

export function isUnsettledOutcome(outcome: TrialOutcome): boolean {
  return UNSETTLED_OUTCOMES.has(outcome);
}

export function pairedFromComparison(c: BatchComparison): PairedDifference {
  return {
    paired: c.paired,
    unpaired: c.unpaired,
    difference: c.difference,
    difference_95: c.difference_95,
    verdict: c.verdict,
    better: c.candidate_better,
    worse: c.reference_better,
    tied: c.tied,
  };
}

export function batchBlockedBy(testCases: number, trials: number): string | undefined {
  if (testCases === 0) return "先勾選至少一題測試題。";
  if (testCases > MAX_TEST_CASES) return `一個批次最多 ${MAX_TEST_CASES} 題，請少勾幾題。`;
  if (!Number.isInteger(trials) || trials < 1 || trials > MAX_TRIALS)
    return `每題要跑 1 到 ${MAX_TRIALS} 次。`;
  return undefined;
}

export function runsToStart(testCases: number, trials: number, withheld: boolean): number {
  return testCases * trials * (withheld ? 2 : 1);
}

export function percent(rate: number): string {
  return `${Math.round(rate * 100)}%`;
}

export function signedPoints(difference: number): string {
  const points = Math.round(difference * 100);
  return `${points > 0 ? "+" : ""}${points} 個百分點`;
}

export function reportStillMoving(report: BatchReport): boolean {
  if (report.batch.status === "launching") return true;
  if (report.batch.status !== "launched") return false;
  const arms = [report, ...(report.skill_withheld ? [report.skill_withheld] : [])];
  return arms.some((arm) =>
    arm.test_cases.some((tc) => tc.trials.some((t) => isUnsettledOutcome(t.outcome))),
  );
}

function isOptionalRate(value: unknown): boolean {
  return value === undefined || (typeof value === "number" && Number.isFinite(value));
}

function isOptionalInterval(value: unknown): boolean {
  return (
    value === undefined ||
    (isRecord(value) && typeof value.low === "number" && typeof value.high === "number")
  );
}

function isOptionalFlag(value: unknown): boolean {
  return value === undefined || typeof value === "boolean";
}

export function isEvaluationBatch(value: unknown, skillId: string): value is EvaluationBatch {
  return (
    isRecord(value) &&
    typeof value.id === "string" &&
    typeof value.skill_id === "string" &&
    value.skill_id.toLowerCase() === skillId.toLowerCase() &&
    typeof value.skill_version_id === "string" &&
    isCount(value.trials) &&
    typeof value.with_skill_withheld === "boolean" &&
    typeof value.status === "string" &&
    STATUSES.has(value.status) &&
    isOptionalText(value.stop_reason) &&
    typeof value.created_at === "string"
  );
}

export function isBatchList(
  value: unknown,
  skillId: string,
): value is { batches: EvaluationBatch[] } {
  return (
    isRecord(value) &&
    Array.isArray(value.batches) &&
    value.batches.every((batch) => isEvaluationBatch(batch, skillId))
  );
}

function isTrial(value: unknown): value is BatchTrial {
  return (
    isRecord(value) &&
    isCount(value.trial) &&
    isOptionalText(value.run_id) &&
    isOptionalText(value.end_run_id) &&
    typeof value.outcome === "string" &&
    OUTCOMES.has(value.outcome)
  );
}

function isTestCase(value: unknown): value is BatchTestCase {
  return (
    isRecord(value) &&
    typeof value.test_case_id === "string" &&
    Array.isArray(value.trials) &&
    value.trials.every(isTrial) &&
    isCount(value.passed) &&
    isCount(value.judged) &&
    isOptionalFlag(value.pass_at_k) &&
    isOptionalFlag(value.pass_all_k) &&
    typeof value.prompt_varied_across === "boolean"
  );
}

function isArm(value: unknown): value is BatchArm {
  return (
    isRecord(value) &&
    Array.isArray(value.test_cases) &&
    value.test_cases.every(isTestCase) &&
    isCount(value.passed) &&
    isCount(value.judged) &&
    isOptionalRate(value.pass_rate) &&
    isOptionalInterval(value.pass_rate_95) &&
    isCount(value.test_cases_all_passed) &&
    isCount(value.test_cases_decided)
  );
}

function isPairedDifference(value: unknown): value is PairedDifference {
  return (
    isRecord(value) &&
    isCount(value.paired) &&
    isCount(value.unpaired) &&
    isOptionalRate(value.difference) &&
    isOptionalInterval(value.difference_95) &&
    isVerdict(value.verdict) &&
    isCount(value.better) &&
    isCount(value.worse) &&
    isCount(value.tied)
  );
}

export function isBatchReport(
  value: unknown,
  skillId: string,
  batchId: string,
): value is BatchReport {
  if (!isRecord(value) || !isArm(value)) return false;
  const report = value as Record<string, unknown>;
  return (
    isEvaluationBatch(report.batch, skillId) &&
    report.batch.id.toLowerCase() === batchId.toLowerCase() &&
    (report.skill_withheld === undefined || isArm(report.skill_withheld)) &&
    (report.skill_effect === undefined || isPairedDifference(report.skill_effect))
  );
}

export function isBatchComparison(
  value: unknown,
  skillId: string,
  batchId: string,
  against: string,
): value is BatchComparison {
  return (
    isRecord(value) &&
    isEvaluationBatch(value.candidate, skillId) &&
    value.candidate.id.toLowerCase() === batchId.toLowerCase() &&
    isEvaluationBatch(value.reference, skillId) &&
    value.reference.id.toLowerCase() === against.toLowerCase() &&
    isCount(value.paired) &&
    isCount(value.unpaired) &&
    isOptionalRate(value.difference) &&
    isOptionalInterval(value.difference_95) &&
    isVerdict(value.verdict) &&
    isCount(value.candidate_better) &&
    isCount(value.reference_better) &&
    isCount(value.tied)
  );
}

export function unconfirmedTestCases(body: unknown): string[] | undefined {
  if (!isRecord(body) || !Array.isArray(body.unconfirmed_test_case_ids)) return undefined;
  const ids = body.unconfirmed_test_case_ids.filter((id): id is string => typeof id === "string");
  return ids.length > 0 ? ids : undefined;
}

export type DifferenceVerdict = CandidatePairedVerdictVerdictEnum;

const VERDICTS = new Set<string>([
  "improved",
  "likely_improved",
  "no_clear_difference",
  "likely_worse",
  "worse",
  "too_few_pairs",
]);

export function verdictSentence(
  verdict: DifferenceVerdict,
  betterLabel: string,
  worseLabel: string,
): string {
  switch (verdict) {
    case "improved":
      return `${betterLabel}，差距明顯`;
    case "likely_improved":
      return `${betterLabel}，但差距只勉強排除巧合`;
    case "likely_worse":
      return `${worseLabel}，但差距只勉強排除巧合`;
    case "worse":
      return `${worseLabel}，差距明顯`;
    case "too_few_pairs":
      return "配對的題目不到兩題，下不了結論";
    case "no_clear_difference":
      return "看不出差別：差距可能只是巧合，不能說哪一邊比較好";
  }
}

export function isVerdict(value: unknown): value is DifferenceVerdict {
  return typeof value === "string" && VERDICTS.has(value);
}
