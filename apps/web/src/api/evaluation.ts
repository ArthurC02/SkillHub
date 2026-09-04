import { useQuery, useQueryClient } from "@tanstack/react-query";
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
  /**
   * `unreported` travels with a null `evaluation_usd` and takes neither of the
   * other two labels — the server sent it long before the contract listed it,
   * and the page's `else` branch called it 「模型閘道實付」 (04 丙-147).
   */
  source: "gateway" | "estimated" | "unreported";
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
 * How many 404s the page will wait through before it stops asking. 20 × 3s ≈
 * 一分鐘 — long enough for a queued `evaluate_run` to be picked up, short enough
 * that a March run nobody ever evaluated does not 404 every three seconds for as
 * long as the tab stays open.
 *
 * Counted with `errorUpdateCount` and **not** `fetchFailureCount`: query-core's
 * `fetchState` zeroes the latter at the start of every fetch, so with
 * `retry: false` it never gets past 1 and a bound written against it does not
 * bound anything. A verdict that does arrive leaves this branch entirely — the
 * poll then follows `status === "pending"` instead.
 *
 * When it runs out the copy has to change with it: EvaluationPanel says the page
 * has stopped asking, because 「結果會自己出現在這裡」 is a promise the code no
 * longer keeps.
 */
export const EVALUATION_POLL_MAX_404 = 20;

/**
 * The same bound for the other unbounded case, and deliberately **not** the same
 * mechanism.
 *
 * They cannot share a counter: a `pending` read is a *success*, so it moves
 * `dataUpdateCount` and never touches the `errorUpdateCount` the 404 branch
 * counts. They also do not answer the same question — 404 is 「沒有人評過這個
 * Run」, `pending` is 「有人開始了，而它卡住了」 — and the sentence each one gives
 * up with is therefore a different sentence.
 *
 * 100 × 3s ≈ 五分鐘, taken from the server's own ceiling rather than from a feel:
 * one attempt of `evaluate_run` is bounded by `judgeTimeout` + `suggestTimeout`
 * (120s + 120s, apps/platform/internal/trial/improvement), and the second attempt
 * marks that same revision `failed` instead of judging again. A row still saying
 * `pending` well past that is a worker that died or a job nobody will retry: the
 * verdict is not on its way, and asking for it every three seconds until the tab
 * closes does not bring it.
 */
export const EVALUATION_POLL_MAX_PENDING = 100;

function evaluationKey(runId: string, revision?: string) {
  return ["evaluation", runId, revision ?? "current"];
}

/**
 * `retry: false` because 404 here means 「未評估」, which no amount of retrying
 * fixes and which the caller renders as a state rather than as a failure.
 *
 * Returns `pendingPollStopped` alongside the query because the view has to change
 * its copy when the pending poll gives up, and nothing on a `UseQueryResult` can
 * tell it that: `dataUpdateCount` lives on the query's state and is not part of
 * the observer result, unlike the `errorUpdateCount` the 404 branch reuses. The
 * spread is load-bearing too — reading every field marks them all tracked, and
 * without that this component never re-renders at all while polling, because a
 * `pending` payload is byte-identical every three seconds and query-core notifies
 * only on tracked props that changed. The copy would then keep promising a
 * re-check that had already stopped, which is the defect being fixed.
 */
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
