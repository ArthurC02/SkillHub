import type {
  BatchReflectionOutcomeEnum,
  ReflectionCandidateFocusEnum,
  ReflectionCandidateOutcomeEnum,
} from "@skillhub/api-client-ts";
import { contractEnum } from "../../../core/api/contract-enum";
import { NONE_KEPT_SENTENCE, isSuggestion, type ImprovementSuggestion } from "../../runs";
import {
  isUnsettledOutcome,
  isVerdict,
  type BatchTestCase,
  type DifferenceVerdict,
} from "./batches.model";
import { isCount, isRecord } from "../../../core/api/wire";

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
  candidates: ReflectionCandidate[];
  evaluation: ReflectionEvaluation | null;
};

export type PairedVerdict = {
  verdict: DifferenceVerdict;
  mean_difference: number | null;
  low: number | null;
  high: number | null;
};

export type CandidateStanding = {
  candidate: number;
  skill_version_id: string | null;
  batch_id: string | null;
  skipped_suggestion_ids: string[];
  settled: boolean;
  kept: boolean;
  dev: PairedVerdict;
  held_out: PairedVerdict;
};

export type ReflectionAdoption = {
  candidate: number;
  skill_version_id: string;
  adopted_at: string;
};

export type ReflectionEvaluation = {
  candidates: CandidateStanding[];
  recommended: number | null;
  adopted: ReflectionAdoption | null;
};

export type UnconfirmedPair = { skill_version_id: string; test_case_id: string };

export type ReflectionCandidate = {
  focus: ReflectionCandidateFocusEnum;
  outcome: ReflectionCandidateOutcomeEnum;
  suggestion_ids: string[];
};

export const CANDIDATE_FOCUS_LABEL: Record<ReflectionCandidateFocusEnum, string> = {
  instructions: "先看指示",
  scripts_and_tools: "先看 Script 與工具",
  references_and_examples: "先看參考資料與範例",
  free: "不限方向",
};

const CANDIDATE_OUTCOMES = contractEnum<ReflectionCandidateOutcomeEnum>({
  unavailable: true,
  none_proposed: true,
  none_kept: true,
  kept: true,
});

function isCandidate(value: unknown): value is ReflectionCandidate {
  return (
    isRecord(value) &&
    Object.hasOwn(CANDIDATE_FOCUS_LABEL, value.focus as string) &&
    CANDIDATE_OUTCOMES.includes(value.outcome as string) &&
    Array.isArray(value.suggestion_ids) &&
    value.suggestion_ids.every((id) => typeof id === "string")
  );
}

export const REFLECTION_OUTCOME_SENTENCE: Record<ReflectionOutcome, string> = {
  restricted: "這個小工具的來源授權還在審查中，審查期間不找改進，這次沒有呼叫模型。",
  not_needed: "開發題的試跑都符合，也沒有需要處理的發現，這次沒有呼叫模型。",
  no_evidence: "開發題失敗的試跑沒有可以引用的證據；平台不提出沒有根據的建議，這次沒有呼叫模型。",
  unavailable:
    "這次找改進時模型沒有回應，所以沒有建議；這不代表小工具沒有可以改進的地方，可以再試一次。",
  none_proposed: "模型讀過開發題的失敗，沒有提出改善建議。",
  none_kept: NONE_KEPT_SENTENCE,
  kept: "",
};

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
    value.suggestions.every(isSuggestion) &&
    Array.isArray(value.candidates) &&
    value.candidates.every(isCandidate) &&
    (value.evaluation === null || isEvaluation(value.evaluation))
  );
}

const isNumberOrNull = (value: unknown) => value === null || typeof value === "number";
const isIdOrNull = (value: unknown) => value === null || typeof value === "string";
const isCandidateIndex = (value: unknown) =>
  typeof value === "number" && Number.isInteger(value) && value >= 0 && value <= 3;

function isPaired(value: unknown): value is PairedVerdict {
  return (
    isRecord(value) &&
    isVerdict(value.verdict) &&
    isNumberOrNull(value.mean_difference) &&
    isNumberOrNull(value.low) &&
    isNumberOrNull(value.high)
  );
}

function isStanding(value: unknown): value is CandidateStanding {
  return (
    isRecord(value) &&
    isCandidateIndex(value.candidate) &&
    isIdOrNull(value.skill_version_id) &&
    isIdOrNull(value.batch_id) &&
    Array.isArray(value.skipped_suggestion_ids) &&
    value.skipped_suggestion_ids.every((id) => typeof id === "string") &&
    typeof value.settled === "boolean" &&
    typeof value.kept === "boolean" &&
    isPaired(value.dev) &&
    isPaired(value.held_out)
  );
}

function isAdoption(value: unknown): value is ReflectionAdoption {
  return (
    isRecord(value) &&
    isCandidateIndex(value.candidate) &&
    typeof value.skill_version_id === "string" &&
    typeof value.adopted_at === "string"
  );
}

function isEvaluation(value: unknown): value is ReflectionEvaluation {
  return (
    isRecord(value) &&
    Array.isArray(value.candidates) &&
    value.candidates.every(isStanding) &&
    (value.recommended === null || isCandidateIndex(value.recommended)) &&
    (value.adopted === null || isAdoption(value.adopted))
  );
}

export function unconfirmedPairs(body: unknown): UnconfirmedPair[] | undefined {
  if (!isRecord(body) || !Array.isArray(body.unconfirmed_pairs)) return undefined;
  const pairs = body.unconfirmed_pairs.filter(
    (p): p is UnconfirmedPair =>
      isRecord(p) && typeof p.skill_version_id === "string" && typeof p.test_case_id === "string",
  );
  return pairs.length > 0 ? pairs : undefined;
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
      tc.trials.some((trial) => isUnsettledOutcome(trial.outcome)),
  );
  return unsettled ? "勾選的開發題還有試跑沒跑完或還沒評估完，等它們都有結果再找改進。" : undefined;
}

export function heldOutTestCaseIds(testCaseIds: string[], reflection: BatchReflection): string[] {
  const dev = new Set(reflection.dev_test_case_ids.map((id) => id.toLowerCase()));
  return testCaseIds.filter((id) => !dev.has(id.toLowerCase()));
}
