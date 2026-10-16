import { useEffect, useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ApiError,
  isWriteResultUnknown as isApplyResultUnknown,
  apiFetch,
  internalAPIPath,
  isLastingReadFailure,
} from "../../core/api/client";
import { queryKeys } from "../../core/api/queryKeys";
import { isEvaluationResponse, isEvaluationRevisionsResponse } from "./evaluation-response.model";

export type EvaluationVerdict = "met" | "partially_met" | "not_met" | "undetermined";
export type CriterionVerdict = "passed" | "failed" | "undetermined";
export type VerdictSource = "rule" | "model" | "user";

export type EvidenceMatch = "exact" | "normalized" | "not_found" | "not_checked";

export type EvidenceRef = {
  kind: "trace_event" | "artifact" | "agent_output";
  match?: EvidenceMatch;
  reattributed_from?: "trace_event" | "artifact" | "agent_output";
  trace_event_id?: string;
  occurred_at?: string;
  artifact_path?: string;
  byte_range?: { start: number; end: number };
  char_range?: { start: number; end: number };
  excerpt: string;
  excerpt_truncated: boolean;
  available: boolean;
};

export type CriterionResult = {
  criterion_id: string;
  text: string;
  result: CriterionVerdict;
  source: VerdictSource;
  evidence: EvidenceRef[];
  reason: string;
  steps?: Array<{
    path: string;
    kind: string;
    result: "passed" | "failed" | "undetermined";
    reason: string;
  }>;
};

export type DeterministicFinding = {
  category: "spec" | "activation" | "execution" | "effect" | "compatibility" | "cost";
  severity: "error" | "warning" | "info";
  message: string;
  evidence: EvidenceRef[];
};

export type EvaluationCost = {
  evaluation_credits: number | null;
  source: "gateway" | "estimated" | "unreported";
  note: string;
};

export type Evaluation = {
  evaluation_id: string;
  run_id: string;
  status: "pending" | "completed" | "failed";
  overall: EvaluationVerdict;
  summary?: string;
  criterion_results: CriterionResult[];
  deterministic_findings: DeterministicFinding[];
  judge_model: string;
  judge_prompt_version: string;
  rubric_version?: string;
  evidence_complete: boolean;
  cost: EvaluationCost;
  feedback?: { helpful: boolean; comment?: string; submitted_at: string };
  evaluated_at: string | null;
  superseded_at?: string | null;
};

export type EvaluationRevision = {
  evaluation_id: string;
  judge_prompt_version: string;
  rubric_version?: string;
  overall: EvaluationVerdict;
  evaluated_at: string | null;
  superseded_at: string | null;
};

export type SuggestionBlockedReason =
  | "path_out_of_bounds"
  | "target_changed"
  | "validation_blocked"
  | "access_restricted"
  | "diff_unavailable";

export type SuggestionDecision = "pending" | "accepted" | "rejected";

export type ImprovementSuggestion = {
  suggestion_id: string;
  category: "skill" | "runtime" | "mcp" | "tool" | "dataset";
  problem: string;
  evidence: EvidenceRef[];
  target_path: string;
  expected_impact: string;
  decision: SuggestionDecision;
  decided_at?: string;
  applied_skill_version_id?: string;
};

export type RunSuggestions = {
  evaluation_id: string;
  suggestions: ImprovementSuggestion[];
};

export type SuggestionDiff = {
  target_path: string;
  unified_diff?: string;
  applicable: boolean;
  blocked_reason?: SuggestionBlockedReason;
};

export type RejectedSuggestion = {
  suggestion_id: string;
  blocked_reason: SuggestionBlockedReason;
  message: string;
};

export type VersionFromSuggestions = {
  skill_id: string;
  version_id: string;
  version_number: number;
  content_hash: string;
  duplicate: boolean;
  applied_suggestion_ids: string[];
  rejected_suggestions: RejectedSuggestion[];
};

export type ComparisonSide = {
  run_id: string;
  skill_id: string;
  skill_version_id: string;
  test_case_id?: string;
  status: string;
  evaluation?: {
    evaluation_id: string;
    status: "pending" | "completed" | "failed";
    overall: EvaluationVerdict;
    cost: EvaluationCost;
  };
  final_output?: string;
  errors?: { category?: string; code?: string; message?: string }[];
  duration_ms?: number;
  cost: {
    credits: number | null;
    is_lower_bound: boolean;
    authoritative_source: string;
  };
  inputs_available: boolean;
};

export type RunComparison = {
  runs: ComparisonSide[];
  criterion_matrix: {
    criterion_id: string;
    text: string;
    results: {
      run_id: string;
      result: CriterionVerdict | null;
      source?: VerdictSource;
    }[];
  }[];
  version_diff_url?: string;
};

export async function getEvaluation(runId: string, revision?: string): Promise<Evaluation> {
  const query = revision ? `?revision=${encodeURIComponent(revision)}` : "";
  const result = await apiFetch<unknown>(`/runs/${encodeURIComponent(runId)}/evaluation${query}`);
  if (!isEvaluationResponse(result, runId, revision))
    throw new Error("Invalid evaluation response");
  return result;
}

export const EVALUATION_POLL_MAX_404 = 20;

export const EVALUATION_POLL_MAX_PENDING = 100;

function evaluationReadCount(
  state: { dataUpdateCount: number; errorUpdateCount: number } | undefined,
) {
  return (state?.dataUpdateCount ?? 0) + (state?.errorUpdateCount ?? 0);
}

export function useEvaluation(runId: string, revision?: string, awaitCurrent = false) {
  const client = useQueryClient();
  const key = queryKeys.evaluation.revision(runId, revision);
  const query = useQuery({
    queryKey: key,
    queryFn: () => getEvaluation(runId, revision),
    enabled: runId.length > 0,
    refetchInterval: (query) => {
      if (revision || !awaitCurrent) return false;
      const error = query.state.error;
      if (
        error instanceof ApiError &&
        (error.status === 401 || (query.state.data && isLastingReadFailure(error)))
      )
        return false;
      if (query.state.data?.status === "pending") {
        return evaluationReadCount(query.state) < EVALUATION_POLL_MAX_PENDING ? 3000 : false;
      }
      if (!(error instanceof ApiError) || error.status !== 404) return false;
      return query.state.errorUpdateCount < EVALUATION_POLL_MAX_404 ? 3000 : false;
    },
  });
  useRevisionsFollowCurrent(
    runId,
    revision || !query.data ? undefined : `${query.data.evaluation_id}:${query.data.status}`,
  );
  const pendingPollStopped =
    !revision &&
    awaitCurrent &&
    query.data?.status === "pending" &&
    evaluationReadCount(client.getQueryState(key)) >= EVALUATION_POLL_MAX_PENDING;
  const initialPollStopped =
    !revision &&
    awaitCurrent &&
    query.data === undefined &&
    query.errorUpdateCount >= EVALUATION_POLL_MAX_404;
  return { ...query, pendingPollStopped, initialPollStopped };
}

function useRevisionsFollowCurrent(runId: string, current: string | undefined) {
  const client = useQueryClient();
  const seen = useRef<{ runId: string; current: string } | undefined>(undefined);
  // Only a changed evaluation or status for the same run invalidates, so
  // StrictMode's second effect run and a switch to another run are no-ops.
  useEffect(() => {
    if (!current) return;
    const last = seen.current;
    if (last && last.runId === runId && last.current !== current) {
      void client.invalidateQueries({ queryKey: queryKeys.evaluation.revisions(runId) });
    }
    seen.current = { runId, current };
  }, [client, runId, current]);
}

export async function getEvaluationRevisions(
  runId: string,
): Promise<{ revisions: EvaluationRevision[] }> {
  const result = await apiFetch<unknown>(`/runs/${encodeURIComponent(runId)}/evaluation/revisions`);
  if (!isEvaluationRevisionsResponse(result))
    throw new Error("Invalid evaluation revisions response");
  return result;
}

export function useEvaluationRevisions(runId: string, evaluated: boolean) {
  return useQuery({
    queryKey: queryKeys.evaluation.revisions(runId),
    queryFn: () => getEvaluationRevisions(runId),
    enabled: runId.length > 0 && evaluated,
  });
}

export async function setEvaluationFeedback(runId: string, helpful: boolean, comment: string) {
  const result = await apiFetch<unknown>(`/runs/${runId}/evaluation/feedback`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ helpful, comment }),
  });
  if (
    !isEvaluationResponse(result, runId) ||
    result.feedback?.helpful !== helpful ||
    (result.feedback?.comment ?? "") !== comment
  )
    throw new Error("Invalid evaluation feedback response");
  return result;
}

export function useEvaluationFeedback(runId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ helpful, comment }: { helpful: boolean; comment: string }) =>
      setEvaluationFeedback(runId, helpful, comment),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.evaluation.run(runId) }),
  });
}

export function useRunSuggestions(runId: string) {
  return useQuery({
    queryKey: queryKeys.evaluation.suggestions(runId),
    queryFn: () => apiFetch<RunSuggestions>(`/runs/${runId}/suggestions`),
    enabled: runId.length > 0,
  });
}

export function useSuggestionDiff(suggestionId: string, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.evaluation.suggestionDiff(suggestionId),
    queryFn: () => apiFetch<SuggestionDiff>(`/suggestions/${suggestionId}/diff`),
    enabled,
    staleTime: 0,
    gcTime: 0,
  });
}

export async function decideSuggestion(suggestionId: string, decision: "accepted" | "rejected") {
  const result = await apiFetch<unknown>(`/suggestions/${suggestionId}/decision`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ decision }),
  });
  if (
    typeof result !== "object" ||
    result === null ||
    !("suggestion_id" in result) ||
    typeof result.suggestion_id !== "string" ||
    result.suggestion_id.toLowerCase() !== suggestionId.toLowerCase() ||
    !("decision" in result) ||
    result.decision !== decision
  )
    throw new Error("Invalid decision response");
  return result;
}

export function useDecideSuggestion(runId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({
      suggestionId,
      decision,
    }: {
      suggestionId: string;
      decision: "accepted" | "rejected";
    }) => decideSuggestion(suggestionId, decision),
    onSuccess: () =>
      client.invalidateQueries({ queryKey: queryKeys.evaluation.suggestions(runId) }),
  });
}

export async function createVersionFromSuggestions(
  skillId: string,
  evaluationId: string,
  suggestionIds: string[],
) {
  const result = await apiFetch<unknown>(`/skills/${skillId}/versions/from-suggestions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ evaluation_id: evaluationId, suggestion_ids: suggestionIds }),
  });
  if (!isAppliedVersion(result)) throw new Error("Invalid applied version response");
  return result;
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === "string" && value.trim().length > 0;
}

function isRejectedSuggestion(value: unknown): value is RejectedSuggestion {
  if (!value || typeof value !== "object") return false;
  const rejected = value as Partial<RejectedSuggestion>;
  return (
    isNonEmptyString(rejected.suggestion_id) &&
    isNonEmptyString(rejected.message) &&
    typeof rejected.blocked_reason === "string" &&
    [
      "path_out_of_bounds",
      "target_changed",
      "validation_blocked",
      "access_restricted",
      "diff_unavailable",
    ].includes(rejected.blocked_reason)
  );
}

export function readRejectedSuggestions(body: unknown): RejectedSuggestion[] | undefined {
  if (!body || typeof body !== "object" || !("rejected_suggestions" in body)) return undefined;
  const rejected = body.rejected_suggestions;
  return Array.isArray(rejected) && rejected.every(isRejectedSuggestion) ? rejected : undefined;
}

function isAppliedVersion(value: unknown): value is VersionFromSuggestions {
  if (!value || typeof value !== "object") return false;
  const result = value as Partial<VersionFromSuggestions>;
  return (
    [result.skill_id, result.version_id, result.content_hash].every(isNonEmptyString) &&
    typeof result.version_number === "number" &&
    Number.isSafeInteger(result.version_number) &&
    result.version_number > 0 &&
    typeof result.duplicate === "boolean" &&
    Array.isArray(result.applied_suggestion_ids) &&
    result.applied_suggestion_ids.length > 0 &&
    result.applied_suggestion_ids.every(isNonEmptyString) &&
    Array.isArray(result.rejected_suggestions) &&
    result.rejected_suggestions.every(isRejectedSuggestion)
  );
}

export { isWriteResultUnknown as isApplyResultUnknown } from "../../core/api/client";

export function useApplySuggestions(runId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({
      skillId,
      evaluationId,
      suggestionIds,
    }: {
      skillId: string;
      evaluationId: string;
      suggestionIds: string[];
    }) => createVersionFromSuggestions(skillId, evaluationId, suggestionIds),
    onSuccess: (created) => {
      client.setQueryData<RunSuggestions>(
        queryKeys.evaluation.suggestions(runId),
        (held) =>
          held && {
            ...held,
            suggestions: held.suggestions.map((s) =>
              created.applied_suggestion_ids.includes(s.suggestion_id)
                ? { ...s, applied_skill_version_id: created.version_id }
                : s,
            ),
          },
      );
      return client.invalidateQueries({ queryKey: queryKeys.skills.versions(created.skill_id) });
    },
    onError: (error, request) => {
      if (isApplyResultUnknown(error)) {
        void client.invalidateQueries({ queryKey: queryKeys.evaluation.suggestions(runId) });
        void client.invalidateQueries({ queryKey: queryKeys.skills.versions(request.skillId) });
      }
    },
  });
}

export function useRunComparison(runId: string, against: string) {
  return useQuery({
    queryKey: queryKeys.evaluation.comparison(runId, against),
    queryFn: () =>
      apiFetch<RunComparison>(`/runs/${runId}/comparison?against=${encodeURIComponent(against)}`),
    enabled: runId.length > 0 && against.length > 0,
  });
}

export function useVersionDiff(url: string | undefined) {
  const path = internalAPIPath(url);
  const query = useQuery({
    queryKey: queryKeys.evaluation.versionDiff(path ?? ""),
    queryFn: () =>
      apiFetch<{ files: { path: string; status: string; diff?: string }[] }>(path as string),
    enabled: Boolean(path),
  });
  return { ...query, invalidURL: Boolean(url) && !path };
}
