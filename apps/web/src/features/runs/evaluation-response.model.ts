import type {
  CheckStepResultEnum,
  CriterionResultResultEnum,
  CriterionResultSourceEnum,
  DeterministicFindingCategoryEnum,
  DeterministicFindingSeverityEnum,
  EvaluationCostSourceEnum,
  EvaluationOverallEnum,
  EvaluationRevisionOverallEnum,
  EvaluationStatusEnum,
  EvidenceRefKindEnum,
  EvidenceRefMatchEnum,
  EvidenceRefReattributedFromEnum,
  ImprovementSuggestionCategoryEnum,
  ImprovementSuggestionDecisionEnum,
  RunComparisonCriterionMatrixInnerResultsInnerResultEnum,
  RunComparisonCriterionMatrixInnerResultsInnerSourceEnum,
  RunComparisonRunsInnerEvaluationOverallEnum,
  RunComparisonRunsInnerEvaluationStatusEnum,
  SuggestionBlockedReason,
} from "@skillhub/api-client-ts";
import { contractEnum } from "../../core/api/contract-enum";
import type {
  Evaluation,
  EvaluationRevision,
  RunComparison,
  RunSuggestions,
  SuggestionDiff,
} from "./evaluation.service";
import { SUGGESTION_OUTCOMES } from "./evaluation.service";

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function isText(value: unknown): value is string {
  return typeof value === "string";
}

function isOptionalText(value: unknown): boolean {
  return value === undefined || isText(value);
}

function isDate(value: unknown): boolean {
  return isText(value) && Number.isFinite(Date.parse(value));
}

function isNullableDate(value: unknown): boolean {
  return value === null || isDate(value);
}

function isMember(value: unknown, values: readonly string[]): boolean {
  return isText(value) && values.includes(value);
}

const EVIDENCE_KINDS = contractEnum<EvidenceRefKindEnum>({
  trace_event: true,
  artifact: true,
  agent_output: true,
});
const EVIDENCE_MATCHES = contractEnum<EvidenceRefMatchEnum>({
  exact: true,
  normalized: true,
  not_found: true,
  not_checked: true,
});
const EVIDENCE_REATTRIBUTIONS = contractEnum<EvidenceRefReattributedFromEnum>({
  trace_event: true,
  artifact: true,
  agent_output: true,
});
const STEP_RESULTS = contractEnum<CheckStepResultEnum>({
  passed: true,
  failed: true,
  undetermined: true,
});
const CRITERION_RESULTS = contractEnum<CriterionResultResultEnum>({
  passed: true,
  failed: true,
  undetermined: true,
});
const CRITERION_SOURCES = contractEnum<CriterionResultSourceEnum>({
  rule: true,
  model: true,
  user: true,
});
const FINDING_CATEGORIES = contractEnum<DeterministicFindingCategoryEnum>({
  spec: true,
  activation: true,
  execution: true,
  effect: true,
  compatibility: true,
  cost: true,
});
const FINDING_SEVERITIES = contractEnum<DeterministicFindingSeverityEnum>({
  error: true,
  warning: true,
  info: true,
});
const COST_SOURCES = contractEnum<EvaluationCostSourceEnum>({
  gateway: true,
  estimated: true,
  unreported: true,
});
const COMPARED_EVALUATION_STATUSES = contractEnum<RunComparisonRunsInnerEvaluationStatusEnum>({
  pending: true,
  completed: true,
  failed: true,
});
const COMPARED_EVALUATION_OVERALLS = contractEnum<RunComparisonRunsInnerEvaluationOverallEnum>({
  met: true,
  partially_met: true,
  not_met: true,
  undetermined: true,
});
const COMPARED_CRITERION_RESULTS =
  contractEnum<RunComparisonCriterionMatrixInnerResultsInnerResultEnum>({
    passed: true,
    failed: true,
    undetermined: true,
  });
const COMPARED_CRITERION_SOURCES =
  contractEnum<RunComparisonCriterionMatrixInnerResultsInnerSourceEnum>({
    rule: true,
    model: true,
    user: true,
  });
const EVALUATION_STATUSES = contractEnum<EvaluationStatusEnum>({
  pending: true,
  completed: true,
  failed: true,
});
const EVALUATION_OVERALLS = contractEnum<EvaluationOverallEnum>({
  met: true,
  partially_met: true,
  not_met: true,
  undetermined: true,
});
const REVISION_OVERALLS = contractEnum<EvaluationRevisionOverallEnum>({
  met: true,
  partially_met: true,
  not_met: true,
  undetermined: true,
});
const SUGGESTION_CATEGORIES = contractEnum<ImprovementSuggestionCategoryEnum>({
  skill: true,
  runtime: true,
  mcp: true,
  tool: true,
  dataset: true,
});
const SUGGESTION_DECISIONS = contractEnum<ImprovementSuggestionDecisionEnum>({
  pending: true,
  accepted: true,
  rejected: true,
});
const SUGGESTION_BLOCKED_REASONS = contractEnum<SuggestionBlockedReason>({
  path_out_of_bounds: true,
  target_changed: true,
  validation_blocked: true,
  access_restricted: true,
  diff_unavailable: true,
});

function isRange(value: unknown): boolean {
  return isRecord(value) && Number.isSafeInteger(value.start) && Number.isSafeInteger(value.end);
}

function isEvidence(value: unknown): boolean {
  if (!isRecord(value)) return false;
  return (
    isMember(value.kind, EVIDENCE_KINDS) &&
    isText(value.excerpt) &&
    typeof value.excerpt_truncated === "boolean" &&
    typeof value.available === "boolean" &&
    (value.match === undefined || isMember(value.match, EVIDENCE_MATCHES)) &&
    (value.reattributed_from === undefined ||
      isMember(value.reattributed_from, EVIDENCE_REATTRIBUTIONS)) &&
    isOptionalText(value.trace_event_id) &&
    isOptionalText(value.artifact_path) &&
    (value.occurred_at === undefined || isDate(value.occurred_at)) &&
    (value.byte_range === undefined || isRange(value.byte_range)) &&
    (value.char_range === undefined || isRange(value.char_range))
  );
}

function isStep(value: unknown): boolean {
  return (
    isRecord(value) &&
    isText(value.path) &&
    isText(value.kind) &&
    isText(value.reason) &&
    isMember(value.result, STEP_RESULTS)
  );
}

function isCriterion(value: unknown): boolean {
  if (!isRecord(value)) return false;
  return (
    isText(value.criterion_id) &&
    isText(value.text) &&
    isText(value.reason) &&
    isMember(value.result, CRITERION_RESULTS) &&
    isMember(value.source, CRITERION_SOURCES) &&
    Array.isArray(value.evidence) &&
    value.evidence.every(isEvidence) &&
    (value.steps === undefined || (Array.isArray(value.steps) && value.steps.every(isStep)))
  );
}

function isFinding(value: unknown): boolean {
  return (
    isRecord(value) &&
    isMember(value.category, FINDING_CATEGORIES) &&
    isMember(value.severity, FINDING_SEVERITIES) &&
    isText(value.message) &&
    Array.isArray(value.evidence) &&
    value.evidence.every(isEvidence)
  );
}

function isCost(value: unknown): boolean {
  return (
    isRecord(value) &&
    (value.evaluation_credits === null || Number.isSafeInteger(value.evaluation_credits)) &&
    isMember(value.source, COST_SOURCES) &&
    isText(value.note)
  );
}

function isComparisonEvaluation(value: unknown): boolean {
  return (
    isRecord(value) &&
    isText(value.evaluation_id) &&
    value.evaluation_id.trim().length > 0 &&
    isMember(value.status, COMPARED_EVALUATION_STATUSES) &&
    isMember(value.overall, COMPARED_EVALUATION_OVERALLS) &&
    isCost(value.cost)
  );
}

function isComparisonRunCost(value: unknown): boolean {
  return (
    isRecord(value) &&
    (value.credits === null || Number.isSafeInteger(value.credits)) &&
    value.is_lower_bound === true &&
    isText(value.authoritative_source)
  );
}

function isComparisonSide(value: unknown, runId: string): boolean {
  if (!isRecord(value)) return false;
  return (
    isText(value.run_id) &&
    value.run_id.toLowerCase() === runId.toLowerCase() &&
    isText(value.skill_id) &&
    value.skill_id.trim().length > 0 &&
    isText(value.skill_version_id) &&
    value.skill_version_id.trim().length > 0 &&
    isOptionalText(value.test_case_id) &&
    isText(value.status) &&
    (value.evaluation === undefined || isComparisonEvaluation(value.evaluation)) &&
    isOptionalText(value.final_output) &&
    (value.duration_ms === undefined || Number.isSafeInteger(value.duration_ms)) &&
    typeof value.inputs_available === "boolean" &&
    isComparisonRunCost(value.cost) &&
    (value.errors === undefined ||
      (Array.isArray(value.errors) &&
        value.errors.every(
          (error) =>
            isRecord(error) &&
            isOptionalText(error.category) &&
            isOptionalText(error.code) &&
            isOptionalText(error.message),
        )))
  );
}

function isComparisonCriterion(value: unknown, runIds: string[]): boolean {
  return (
    isRecord(value) &&
    isText(value.criterion_id) &&
    isText(value.text) &&
    Array.isArray(value.results) &&
    value.results.length === 2 &&
    value.results.every(
      (result, index) =>
        isRecord(result) &&
        isText(result.run_id) &&
        result.run_id.toLowerCase() === runIds[index].toLowerCase() &&
        (result.result === null || isMember(result.result, COMPARED_CRITERION_RESULTS)) &&
        (result.source === undefined || isMember(result.source, COMPARED_CRITERION_SOURCES)),
    )
  );
}

export function isRunComparisonResponse(
  value: unknown,
  runId: string,
  against: string,
): value is RunComparison {
  const runIds = [runId, against];
  return (
    isRecord(value) &&
    Array.isArray(value.runs) &&
    value.runs.length === 2 &&
    value.runs.every((side, index) => isComparisonSide(side, runIds[index])) &&
    Array.isArray(value.criterion_matrix) &&
    value.criterion_matrix.every((row) => isComparisonCriterion(row, runIds)) &&
    isOptionalText(value.version_diff_url)
  );
}

function isFeedback(value: unknown): boolean {
  return (
    isRecord(value) &&
    typeof value.helpful === "boolean" &&
    isOptionalText(value.comment) &&
    isDate(value.submitted_at)
  );
}

function matchesEvaluationIdentity(
  value: Record<string, unknown>,
  runId: string,
  revision?: string,
): boolean {
  return (
    isText(value.evaluation_id) &&
    value.evaluation_id.trim().length > 0 &&
    isText(value.run_id) &&
    value.run_id.toLowerCase() === runId.toLowerCase() &&
    (!revision || value.evaluation_id.toLowerCase() === revision.toLowerCase())
  );
}

export function isEvaluationResponse(
  value: unknown,
  runId: string,
  revision?: string,
): value is Evaluation {
  if (!isRecord(value)) return false;
  return (
    matchesEvaluationIdentity(value, runId, revision) &&
    isMember(value.status, EVALUATION_STATUSES) &&
    isMember(value.overall, EVALUATION_OVERALLS) &&
    isOptionalText(value.summary) &&
    isOptionalText(value.rubric_version) &&
    isText(value.judge_model) &&
    isText(value.judge_prompt_version) &&
    typeof value.evidence_complete === "boolean" &&
    isNullableDate(value.evaluated_at) &&
    (value.superseded_at === undefined || isNullableDate(value.superseded_at)) &&
    isCost(value.cost) &&
    (value.feedback === undefined || isFeedback(value.feedback)) &&
    Array.isArray(value.criterion_results) &&
    value.criterion_results.every(isCriterion) &&
    Array.isArray(value.deterministic_findings) &&
    value.deterministic_findings.every(isFinding)
  );
}

function isRevision(value: unknown): boolean {
  return (
    isRecord(value) &&
    isText(value.evaluation_id) &&
    value.evaluation_id.trim().length > 0 &&
    isText(value.judge_prompt_version) &&
    isOptionalText(value.rubric_version) &&
    isMember(value.overall, REVISION_OVERALLS) &&
    isNullableDate(value.evaluated_at) &&
    isNullableDate(value.superseded_at)
  );
}

export function isEvaluationRevisionsResponse(
  value: unknown,
): value is { revisions: EvaluationRevision[] } {
  return isRecord(value) && Array.isArray(value.revisions) && value.revisions.every(isRevision);
}

function isSuggestion(value: unknown): boolean {
  if (!isRecord(value)) return false;
  return (
    isText(value.suggestion_id) &&
    value.suggestion_id.trim().length > 0 &&
    isMember(value.category, SUGGESTION_CATEGORIES) &&
    isText(value.problem) &&
    Array.isArray(value.evidence) &&
    value.evidence.every(isEvidence) &&
    isText(value.target_path) &&
    isText(value.expected_impact) &&
    isMember(value.decision, SUGGESTION_DECISIONS) &&
    (value.decided_at === undefined || isDate(value.decided_at)) &&
    (value.applied_skill_version_id === undefined ||
      (isText(value.applied_skill_version_id) && value.applied_skill_version_id.trim().length > 0))
  );
}

export function isRunSuggestionsResponse(value: unknown): value is RunSuggestions {
  return (
    isRecord(value) &&
    isText(value.evaluation_id) &&
    value.evaluation_id.trim().length > 0 &&
    (value.outcome === undefined ||
      (SUGGESTION_OUTCOMES as readonly unknown[]).includes(value.outcome)) &&
    Array.isArray(value.suggestions) &&
    value.suggestions.every(isSuggestion)
  );
}

export function isSuggestionDiffResponse(value: unknown): value is SuggestionDiff {
  return (
    isRecord(value) &&
    isText(value.target_path) &&
    typeof value.applicable === "boolean" &&
    isOptionalText(value.unified_diff) &&
    (value.blocked_reason === undefined ||
      isMember(value.blocked_reason, SUGGESTION_BLOCKED_REASONS))
  );
}
