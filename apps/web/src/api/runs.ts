import type { Labelled } from "./types";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";

/**
 * GET /runs/{id} (contracts/openapi/public.yaml, RUN-002).
 *
 * Only the fields the run screens actually read. `status` is deliberately not
 * among them: the trace summary already carries it and the pages take it from
 * there, so this query stays a one-shot read of things that never change while
 * the page is open.
 *
 * `skill_id` is what the EVAL-002 apply call needs (POST
 * /skills/{id}/versions/from-suggestions); `test_case_id` is the editable draft a
 * re-run would be started from, never the frozen snapshot. Neither is permission:
 * starting a run is still preflight plus a confirmed summary hash (TEST-009).
 */

export type Run = {
  run_id: string;
  skill_id: string;
  skill_version_id: string;
  test_case_snapshot_id: string;
  /** Absent when the draft no longer resolves; a re-run needs it, so guard on it. */
  test_case_id?: string;
};

export function useRun(runId: string) {
  return useQuery({
    queryKey: ["run", runId],
    queryFn: () => apiFetch<Run>(`/runs/${runId}`),
    enabled: runId.length > 0,
    retry: false,
  });
}

export function cancelRun(runId: string) {
  return apiFetch<Run & { note?: string }>(`/runs/${runId}/cancel`, { method: "POST" });
}

/**
 * GET /runs — the workspace's run history (WS-004).
 *
 * A narrower row than `Run` by contract: what happened, to which skill, when.
 * `status` carries the same warning it does everywhere else — `succeeded` says
 * the workload finished, not that the task was done (ADR-025) — so any surface
 * rendering it words it as execution.
 */
export type RunListItem = {
  run_id: string;
  status: string;
  /**
   * The second axis (04 丙-32). Required and never null — a run with no
   * evaluation carries `not_evaluated` / 未評估, because an empty verdict beside
   * a column of 「執行完成」 reads as a pass, which is the misreading ADR-025
   * separates the two axes to prevent. `value` folds the evaluation's own status
   * in, so 「評估中」 and 「評估失敗」 are distinguishable from 「無法判斷」.
   */
  evaluation: Labelled;
  status_reason?: string;
  skill_id: string;
  /** Joined server-side: a history page is the one place N runs render at once. */
  skill_name: string;
  skill_version_id: string;
  /** The editable draft a re-run would start from. Absent when it no longer resolves. */
  test_case_id?: string;
  provider: string;
  failure_class?: string;
  /**
   * `{value, label, note}`, not a bare enum (04 丙-29 ②).
   *
   * The value is still the database enum `run_cleanup_status`
   * (db/migrations/0004_test_lab_and_runs.sql:75) — but the words now come from
   * the server, because of how this field failed. The contract said `cleaning`
   * where the database said `cleaning_up`, this union was compiled against the
   * wrong half, and a run that was genuinely mid-teardown rendered its cleanup
   * state as a **blank**. A client-side enum→中文 table can only fail that way;
   * a served label cannot.
   */
  cleanup_status: Labelled;
  created_at: string;
  started_at?: string;
  finished_at?: string;
};

/**
 * `testCaseId` narrows the history to one draft — the 執行歷史 that closes the
 * 建立 → 試跑 → 回來看 loop. Matched against the test case each run's snapshot
 * was frozen from, so a run stays listed after the draft has been edited.
 */
export function useRuns(testCaseId?: string) {
  return useInfiniteQuery({
    queryKey: ["runs", testCaseId ?? ""],
    initialPageParam: 0,
    queryFn: ({ pageParam }) => {
      const params = new URLSearchParams({ limit: "51", offset: String(pageParam) });
      if (testCaseId) params.set("test_case_id", testCaseId);
      return apiFetch<{ runs: RunListItem[] }>(`/runs?${params}`).then((page) => ({
        runs: page.runs.slice(0, 50),
        nextOffset: page.runs.length > 50 ? pageParam + 50 : undefined,
      }));
    },
    getNextPageParam: (last) => last.nextOffset,
    retry: false,
  });
}

/**
 * GET /runs/{id}/artifacts — what a run produced, as a manifest (02:SEC-006).
 *
 * File names, sizes and hashes; never the bytes. The archive is a sandbox's
 * output and the control plane does not open it (iron rule 1), so there is no
 * link to serve and this list does not pretend there is one.
 */
export type RunArtifact = {
  artifact_id: string;
  file_name: string;
  content_type: string;
  size_bytes: number;
  content_hash: string;
  created_at: string;
  expires_at?: string;
  /**
   * The stored bytes are gone while the row remains — retention expiry, or a
   * reconciler finding them missing. Distinct from the owner deleting it, which
   * takes the row out of this list entirely.
   */
  purged: boolean;
};

export function useRunArtifacts(runId: string) {
  return useQuery({
    queryKey: ["run", runId, "artifacts"],
    queryFn: () => apiFetch<{ artifacts: RunArtifact[] }>(`/runs/${runId}/artifacts`),
    enabled: runId.length > 0,
    retry: false,
  });
}

/** Idempotent by contract: 204 for an id that is not there, which is not a failure. */
export function deleteRunArtifact(runId: string, artifactId: string) {
  return apiFetch<void>(`/runs/${runId}/artifacts/${artifactId}`, { method: "DELETE" });
}
