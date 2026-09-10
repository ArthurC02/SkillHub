import type { Labelled } from "./types";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";

export type Run = {
  run_id: string;
  skill_id: string;
  skill_version_id: string;
  test_case_snapshot_id: string;
  test_case_id?: string;
  failure_class?: Labelled;
  cleanup_status: Labelled;
  attempts?: Array<{
    run_attempt_id: string;
    attempt_number: number;
    provider: string;
    provider_run_id?: string;
    error_class?: string;
    error_message?: string;
    started_at?: string;
    finished_at?: string;
  }>;
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

export type RunListItem = {
  run_id: string;
  status: string;
  evaluation: Labelled;
  status_reason?: string;
  skill_id: string;
  skill_name: string;
  skill_version_id: string;
  test_case_id?: string;
  provider: string;
  failure_class?: Labelled;
  cleanup_status: Labelled;
  created_at: string;
  started_at?: string;
  finished_at?: string;
};

export function useRuns(testCaseId?: string, enabled = true) {
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
    enabled,
    retry: false,
  });
}

export type RunArtifact = {
  artifact_id: string;
  file_name: string;
  content_type: string;
  size_bytes: number;
  content_hash: string;
  created_at: string;
  expires_at?: string;
  purged: boolean;
};

export type RunArtifacts = { artifacts: RunArtifact[]; truncated: boolean };

export function useRunArtifacts(runId: string) {
  return useQuery({
    queryKey: ["run", runId, "artifacts"],
    queryFn: () => apiFetch<RunArtifacts>(`/runs/${runId}/artifacts`),
    enabled: runId.length > 0,
    retry: false,
  });
}

export function deleteRunArtifact(runId: string, artifactId: string) {
  return apiFetch<void>(`/runs/${runId}/artifacts/${artifactId}`, { method: "DELETE" });
}
