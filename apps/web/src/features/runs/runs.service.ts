import type { Labelled } from "../../core/api/types";
import type { RunStatus } from "./trace.service";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query";
import {
  apiFetch,
  isWriteResultUnknown as isContinuationResultUnknown,
} from "../../core/api/client";
export { isWriteResultUnknown as isContinuationResultUnknown } from "../../core/api/client";
import { queryKeys } from "../../core/api/queryKeys";

export type Run = {
  run_id: string;
  status: RunStatus;
  skill_withheld?: boolean;
  status_reason?: string;
  skill_id: string;
  skill_version_id: string;
  test_case_snapshot_id: string;
  test_case_id?: string;
  provider: string;
  failure_class?: Labelled;
  cleanup_status: Labelled;
  cancel_requested_at?: string;
  created_at: string;
  started_at?: string;
  finished_at?: string;
  transitions?: Array<{
    from_status?: string;
    to_status: string;
    reason?: string;
    occurred_at: string;
  }>;
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
    queryKey: queryKeys.runs.detail(runId),
    queryFn: async () => {
      const run = await apiFetch<Run>(`/runs/${runId}`);
      if (
        !run ||
        typeof run.run_id !== "string" ||
        run.run_id.toLowerCase() !== runId.toLowerCase()
      )
        throw new Error("Invalid Run response");
      return run;
    },
    enabled: runId.length > 0,
  });
}

export function cancelRun(runId: string) {
  return apiFetch<Run & { note?: string }>(`/runs/${runId}/cancel`, { method: "POST" });
}

export function useCancelRun(runId: string) {
  const client = useQueryClient();
  const refreshTrace = () => client.invalidateQueries({ queryKey: queryKeys.trace.run(runId) });
  return useMutation({
    mutationFn: () => cancelRun(runId),
    onSuccess: () =>
      Promise.all([
        refreshTrace(),
        client.invalidateQueries({ queryKey: queryKeys.runs.detail(runId) }),
      ]),
    onError: refreshTrace,
  });
}

export async function continueRun(runId: string, answers: string[]) {
  const next = await apiFetch<unknown>(`/runs/${runId}/continuations`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ answers }),
  });
  if (
    !next ||
    typeof next !== "object" ||
    !("run_id" in next) ||
    typeof next.run_id !== "string" ||
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(next.run_id) ||
    next.run_id.toLowerCase() === runId.toLowerCase()
  )
    throw new Error("Invalid continuation Run response");
  return { run_id: next.run_id };
}

export function useContinueRun(runId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (answers: string[]) => continueRun(runId, answers),
    onSettled: (_result, error) => {
      void client.invalidateQueries({ queryKey: queryKeys.trace.run(runId) });
      if (!error || isContinuationResultUnknown(error)) {
        void client.invalidateQueries({ queryKey: queryKeys.runs.lists });
        void client.invalidateQueries({ queryKey: queryKeys.activity });
        void client.invalidateQueries({ queryKey: queryKeys.credits });
      }
    },
  });
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

function isRunLabel(value: unknown): value is Labelled {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const label = value as Partial<Labelled>;
  return (
    typeof label.value === "string" &&
    typeof label.label === "string" &&
    (label.note === undefined || typeof label.note === "string")
  );
}

function isRunListItem(value: unknown): value is RunListItem {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const run = value as Partial<RunListItem>;
  return (
    typeof run.run_id === "string" &&
    typeof run.status === "string" &&
    typeof run.skill_id === "string" &&
    typeof run.skill_name === "string" &&
    typeof run.skill_version_id === "string" &&
    typeof run.provider === "string" &&
    typeof run.created_at === "string" &&
    isRunLabel(run.evaluation) &&
    isRunLabel(run.cleanup_status) &&
    (run.failure_class === undefined || isRunLabel(run.failure_class)) &&
    [run.status_reason, run.test_case_id, run.started_at, run.finished_at].every(
      (field) => field === undefined || typeof field === "string",
    )
  );
}

type RunListPage = { runs: RunListItem[]; nextOffset: number | undefined };

function uniqueRunPages(data: InfiniteData<RunListPage, number>) {
  const locations = new Map<string, { page: number; row: number; runId: string }>();
  const pages = data.pages.map((item): RunListPage => ({ ...item, runs: [] }));
  data.pages.forEach((item, pageIndex) => {
    item.runs.forEach((run) => {
      const identity = run.run_id.toLowerCase();
      const previous = locations.get(identity);
      if (previous) {
        pages[previous.page].runs[previous.row] = { ...run, run_id: previous.runId };
      } else {
        locations.set(identity, {
          page: pageIndex,
          row: pages[pageIndex].runs.length,
          runId: run.run_id,
        });
        pages[pageIndex].runs.push(run);
      }
    });
  });
  return { ...data, pages };
}

export function useRuns({
  testCaseId,
  skillVersionId,
  enabled = true,
}: {
  testCaseId?: string;
  skillVersionId?: string;
  enabled?: boolean;
} = {}) {
  return useInfiniteQuery({
    queryKey: queryKeys.runs.list(testCaseId, skillVersionId),
    initialPageParam: 0,
    queryFn: ({ pageParam }) => {
      const params = new URLSearchParams({ limit: "51", offset: String(pageParam) });
      if (testCaseId) params.set("test_case_id", testCaseId);
      if (skillVersionId) params.set("skill_version_id", skillVersionId);
      return apiFetch<unknown>(`/runs?${params}`).then((page) => {
        if (
          !page ||
          typeof page !== "object" ||
          Array.isArray(page) ||
          !("runs" in page) ||
          !Array.isArray(page.runs) ||
          !page.runs.every(isRunListItem)
        )
          throw new Error("Invalid Run list response");
        return {
          runs: page.runs.slice(0, 50),
          nextOffset: page.runs.length > 50 ? pageParam + 50 : undefined,
        };
      });
    },
    getNextPageParam: (last) => last.nextOffset,
    select: uniqueRunPages,
    enabled,
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
    queryKey: queryKeys.runs.artifacts(runId),
    queryFn: () => apiFetch<RunArtifacts>(`/runs/${runId}/artifacts`),
    enabled: runId.length > 0,
  });
}

export function deleteRunArtifact(runId: string, artifactId: string) {
  return apiFetch<void>(`/runs/${runId}/artifacts/${artifactId}`, { method: "DELETE" });
}

export function useDeleteRunArtifact(runId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (artifactId: string) => deleteRunArtifact(runId, artifactId),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.runs.artifacts(runId) }),
  });
}
