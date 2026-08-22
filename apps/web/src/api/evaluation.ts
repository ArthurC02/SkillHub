import { useQuery } from "@tanstack/react-query";
import { ApiError, apiFetch } from "./client";

/**
 * The EVAL-001/002/003 read and write surface, exactly as
 * contracts/openapi/public.yaml declares it (implementation rule 12).
 *
 * Two shapes of this file are load-bearing rather than stylistic:
 *
 * 1. **`Evaluation` is a resource of its own, never a field of a run.**
 *    `Run.status` says what happened while the workload executed;
 *    `Evaluation.overall` says whether the task was achieved. Nothing here
 *    merges them, and nothing derives one from the other (ADR-025).
 * 2. **A run with no evaluation answers 404, and that 404 is a state.**
 *    「未評估」 is not an error and not a pass, so the query does not retry it
 *    and the view renders it as its own thing. An empty body would be rendered
 *    by any UI as a clean bill of health, which is why the server does not
 *    send one.
 */

export type EvaluationVerdict = "met" | "partially_met" | "not_met" | "undetermined";
export type CriterionVerdict = "passed" | "failed" | "undetermined";
export type VerdictSource = "rule" | "model" | "user";

/**
 * A citation plus a copy of what it cites. Both, because they expire on
 * different clocks: trace partitions get dropped, evaluation reports get read
 * long afterwards. `available: false` means the original is gone — the excerpt
 * still shows, labelled as the copy kept at judgement time (ADR-026 決策 2).
 */
/**
 * How the platform's own search for the quote ended (ADR-043). Never the
 * judge's answer — a citation holds because its quote is findable, not because
 * of the label it arrived under.
 */
export type EvidenceMatch = "exact" | "normalized" | "not_found" | "not_checked";

export type EvidenceRef = {
  kind: "trace_event" | "artifact" | "agent_output";
  /** Reports written before 2026-08-22 carry no `match`; treat absence as unknown, never as verified. */
  match?: EvidenceMatch;
  /**
   * Present when the quote was found somewhere other than where the judge filed
   * it. `kind` has been corrected; this keeps what it was filed as.
   */
  reattributed_from?: "trace_event" | "artifact" | "agent_output";
  trace_event_id?: string;
  occurred_at?: string;
  artifact_path?: string;
  byte_range?: { start: number; end: number };
  char_range?: { start: number; end: number };
  /** Masked before storage and untrusted: render as inert text, never markup. */
  excerpt: string;
  excerpt_truncated: boolean;
  available: boolean;
};

export type CriterionResult = {
  criterion_id: string;
  text: string;
  result: CriterionVerdict;
  /** Set by the control plane from where the verdict came, never self-reported. */
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
  /** null means the gateway reported nothing. Never render it as 0. */
  evaluation_usd: number | null;
  source: "gateway" | "estimated";
  note: string;
};

export type Evaluation = {
  evaluation_id: string;
  run_id: string;
  /** Whether the judgement itself ran — a different question from `overall`. */
  status: "pending" | "completed" | "failed";
  overall: EvaluationVerdict;
  summary?: string;
  criterion_results: CriterionResult[];
  deterministic_findings: DeterministicFinding[];
  judge_model: string;
  judge_prompt_version: string;
  /** Absent means "no rubric", not "the default rubric". */
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
  /** Model-written. Inert text. */
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
  /** Per side: two runs of different skills may be compared. */
  skill_id: string;
  skill_version_id: string;
  /**
   * The editable test case this side's snapshot was frozen from — what a re-run
   * would address, and not permission to start one. Read with `inputs_available`.
   */
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
    /** null means no usage event carried a cost. Never 0. */
    usd: number | null;
    /** Always true by contract: the sum over trace usage events is a floor. */
    is_lower_bound: boolean;
    authoritative_source: string;
  };
  /** False: these inputs are gone, so no surface may offer a re-run of them. */
  inputs_available: boolean;
};

export type RunComparison = {
  runs: ComparisonSide[];
  criterion_matrix: {
    criterion_id: string;
    text: string;
    results: {
      run_id: string;
      /** null is "no verdict on this side", which is not `undetermined`. */
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

/**
 * `retry: false` because 404 here means 「未評估」, which no amount of retrying
 * fixes and which the caller renders as a state rather than as a failure.
 */
export function useEvaluation(runId: string, revision?: string, awaitCurrent = false) {
  return useQuery({
    queryKey: ["evaluation", runId, revision ?? "current"],
    queryFn: () => getEvaluation(runId, revision),
    enabled: runId.length > 0,
    retry: false,
    refetchInterval: (query) => {
      if (revision || !awaitCurrent) return false;
      if (query.state.data?.status === "pending") return 3000;
      const error = query.state.error;
      return error instanceof ApiError && error.status === 404 ? 3000 : false;
    },
  });
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
    // Re-read every time it is opened: `applicable` is evaluated against the
    // current package and can go stale while the page is open.
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

/**
 * Accepted suggestions become exactly one new immutable version; the version
 * they were written against is untouched (iron rule 4). Creating it is not
 * permission to run it — the content hash changed, so TEST-009 will require a
 * fresh preflight confirmation.
 */
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
