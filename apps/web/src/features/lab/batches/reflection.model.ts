import type { BatchReflectionOutcomeEnum } from "@skillhub/api-client-ts";
import { contractEnum } from "../../../core/api/contract-enum";
import { isSuggestion, type ImprovementSuggestion } from "../../runs";
import type { BatchTestCase, TrialOutcome } from "./batches.model";

export type ReflectionOutcome = BatchReflectionOutcomeEnum;

const REFLECTION_OUTCOMES = contractEnum<ReflectionOutcome>({
  restricted: true,
  not_needed: true,
  no_evidence: true,
  unavailable: true,
  none_proposed: true,
  none_kept: true,
  kept: true,
});

export type BatchReflection = {
  reflection_id: string;
  batch_id: string;
  dev_test_case_ids: string[];
  outcome: ReflectionOutcome;
  runs_read: number;
  runs_not_read: number;
  created_at: string;
  suggestions: ImprovementSuggestion[];
};

export const REFLECTION_OUTCOME_SENTENCE: Record<ReflectionOutcome, string> = {
  restricted: "這個小工具的來源授權還在審查中，審查期間不找改進，這次沒有呼叫模型。",
  not_needed: "開發題的試跑都符合，也沒有需要處理的發現，這次沒有呼叫模型。",
  no_evidence: "開發題失敗的試跑沒有可以引用的證據；平台不提出沒有根據的建議，這次沒有呼叫模型。",
  unavailable:
    "這次找改進時模型沒有回應，所以沒有建議；這不代表小工具沒有可以改進的地方，可以再試一次。",
  none_proposed: "模型讀過開發題的失敗，沒有提出改善建議。",
  none_kept: "模型提出的建議都沒有通過證據核對，平台一條都沒有保留。",
  kept: "",
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

const isCount = (value: unknown) =>
  typeof value === "number" && Number.isSafeInteger(value) && value >= 0;

export function isBatchReflection(value: unknown, batchId: string): value is BatchReflection {
  return (
    isRecord(value) &&
    typeof value.reflection_id === "string" &&
    typeof value.batch_id === "string" &&
    value.batch_id.toLowerCase() === batchId.toLowerCase() &&
    Array.isArray(value.dev_test_case_ids) &&
    value.dev_test_case_ids.every((id) => typeof id === "string") &&
    REFLECTION_OUTCOMES.includes(value.outcome as string) &&
    isCount(value.runs_read) &&
    isCount(value.runs_not_read) &&
    typeof value.created_at === "string" &&
    Array.isArray(value.suggestions) &&
    value.suggestions.every(isSuggestion)
  );
}

export function isBatchReflectionList(
  value: unknown,
  batchId: string,
): value is { reflections: BatchReflection[] } {
  return (
    isRecord(value) &&
    Array.isArray(value.reflections) &&
    value.reflections.every((r) => isBatchReflection(r, batchId))
  );
}

const UNSETTLED: TrialOutcome[] = ["pending", "running", "awaiting_answer"];

export function reflectHoldReason(
  testCases: BatchTestCase[],
  dev: ReadonlySet<string>,
  launching: boolean,
): string | undefined {
  if (launching) return "這一批還在開始中，等它開始跑之後再找改進。";
  if (dev.size === 0) return "先勾至少一題開發題。";
  if (dev.size >= testCases.length) return "至少留一題不勾，之後用它檢查改進有沒有用。";
  const unsettled = testCases.some(
    (tc) =>
      dev.has(tc.test_case_id.toLowerCase()) &&
      tc.trials.some((trial) => UNSETTLED.includes(trial.outcome)),
  );
  return unsettled ? "勾選的開發題還有試跑沒跑完或還沒評估完，等它們都有結果再找改進。" : undefined;
}

export function heldOutTestCaseIds(testCaseIds: string[], reflection: BatchReflection): string[] {
  const dev = new Set(reflection.dev_test_case_ids.map((id) => id.toLowerCase()));
  return testCaseIds.filter((id) => !dev.has(id.toLowerCase()));
}
