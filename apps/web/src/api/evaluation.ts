import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, apiFetch } from "./client";

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
  evaluated_at: string;
  superseded_at?: string | null;
};

export type EvaluationRevision = {
  evaluation_id: string;
  judge_prompt_version: string;
  rubric_version?: string;
  overall: EvaluationVerdict;
  evaluated_at: string;
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

export function getEvaluation(runId: string, revision?: string) {
  const query = revision ? `?revision=${encodeURIComponent(revision)}` : "";
  return apiFetch<Evaluation>(`/runs/${runId}/evaluation${query}`);
}

export const EVALUATION_POLL_MAX_404 = 20;

export const EVALUATION_POLL_MAX_PENDING = 100;

function evaluationKey(runId: string, revision?: string) {
  return ["evaluation", runId, revision ?? "current"];
}

export function useEvaluation(runId: string, revision?: string, awaitCurrent = false) {
  const client = useQueryClient();
  const query = useQuery({
    queryKey: evaluationKey(runId, revision),
    queryFn: () => getEvaluation(runId, revision),
    enabled: runId.length > 0,
    retry: false,
    refetchInterval: (query) => {
      if (revision || !awaitCurrent) return false;
      if (query.state.data?.status === "pending") {
        return query.state.dataUpdateCount < EVALUATION_POLL_MAX_PENDING ? 3000 : false;
      }
      const error = query.state.error;
      if (!(error instanceof ApiError) || error.status !== 404) return false;
      return query.state.errorUpdateCount < EVALUATION_POLL_MAX_404 ? 3000 : false;
    },
  });
  const pendingPollStopped =
    !revision &&
    awaitCurrent &&
    query.data?.status === "pending" &&
    (client.getQueryState(evaluationKey(runId, revision))?.dataUpdateCount ?? 0) >=
      EVALUATION_POLL_MAX_PENDING;
  return { ...query, pendingPollStopped };
}

export function useEvaluationRevisions(runId: string) {
  return useQuery({
    queryKey: ["evaluation", runId, "revisions"],
    queryFn: () =>
      apiFetch<{ revisions: EvaluationRevision[] }>(`/runs/${runId}/evaluation/revisions`),
    enabled: runId.length > 0,
    retry: false,
  });
}

export function setEvaluationFeedback(runId: string, helpful: boolean, comment: string) {
  return apiFetch<Evaluation>(`/runs/${runId}/evaluation/feedback`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ helpful, comment }),
  });
}

export function useRunSuggestions(runId: string) {
  return useQuery({
    queryKey: ["suggestions", runId],
    queryFn: () =>
      apiFetch<{ evaluation_id: string; suggestions: ImprovementSuggestion[] }>(
        `/runs/${runId}/suggestions`,
      ),
    enabled: runId.length > 0,
    retry: false,
  });
}

export function useSuggestionDiff(suggestionId: string, enabled: boolean) {
  return useQuery({
    queryKey: ["suggestion-diff", suggestionId],
    queryFn: () => apiFetch<SuggestionDiff>(`/suggestions/${suggestionId}/diff`),
    enabled,
    retry: false,
    staleTime: 0,
    gcTime: 0,
  });
}

export function decideSuggestion(suggestionId: string, decision: "accepted" | "rejected") {
  return apiFetch<ImprovementSuggestion>(`/suggestions/${suggestionId}/decision`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ decision }),
  });
}

export function createVersionFromSuggestions(
  skillId: string,
  evaluationId: string,
  suggestionIds: string[],
) {
  return apiFetch<VersionFromSuggestions>(`/skills/${skillId}/versions/from-suggestions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ evaluation_id: evaluationId, suggestion_ids: suggestionIds }),
  });
}

export function useRunComparison(runId: string, against: string) {
  return useQuery({
    queryKey: ["comparison", runId, against],
    queryFn: () =>
      apiFetch<RunComparison>(`/runs/${runId}/comparison?against=${encodeURIComponent(against)}`),
    enabled: runId.length > 0 && against.length > 0,
    retry: false,
  });
}

export function useVersionDiff(url: string | undefined) {
  return useQuery({
    queryKey: ["version-diff", url ?? ""],
    queryFn: () =>
      apiFetch<{ files: { path: string; status: string; diff?: string }[] }>(url as string),
    enabled: Boolean(url),
    retry: false,
  });
}
