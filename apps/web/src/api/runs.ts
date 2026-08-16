import { useQuery } from "@tanstack/react-query";
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
