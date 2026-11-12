import type {
  RunPermissionSummaryBlockedEnum,
  RunPermissionSummaryContentScriptsStatusEnum,
} from "@skillhub/api-client-ts";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, apiFetch, isWriteResultUnknown, jsonInit } from "../../core/api/client";
import { queryKeys } from "../../core/api/queryKeys";

export interface PreflightDataset {
  dataset_id: string;
  file_name: string;
  content_type: string;
  size_bytes: number;
  content_hash: string;
}

export interface PreflightSummary {
  skill_version_id: string;
  skill_content_hash: string;
  test_case_id: string;
  datasets: PreflightDataset[];
  dataset_total_bytes: number;
  scripts: { status: RunPermissionSummaryContentScriptsStatusEnum; findings: string[] };
  tools: string[];
  mcp_servers: string[];
  network: { mode: string; allow: string[] };
  injected_secrets: string[];
  provider: {
    name: string;
    isolation_strength?: string;
    rootless: boolean;
    runtime?: string;
    runtime_version?: string;
  };
  resource_limits: {
    vcpu: number;
    memory_bytes: number;
    disk_bytes: number;
    max_pids: number;
    max_open_files: number;
    wall_clock_soft_seconds: number;
    wall_clock_hard_seconds: number;
    artifact_total_bytes: number;
    artifact_file_bytes: number;
    token_budget: {
      max_input_tokens: number;
      max_output_tokens: number;
    };
  };
}

export interface CostEstimate {
  low_credits: number;
  typical_credits: number;
  high_credits: number;
  basis: string;
}

export interface RunQuota {
  remaining_today: number;
  remaining_window: number;
  window_resets_at: string;
  limits: { daily: number; window: number; window_days: number; concurrent: number };
}

export interface PreflightResponse {
  summary: PreflightSummary;
  summary_hash: string;
  estimated_cost: CostEstimate;
  ceiling_credits?: number;
  quota?: RunQuota;
  blocked?: RunPermissionSummaryBlockedEnum;
  notes: string[];
}

export interface DatasetLimits {
  max_file_bytes: number;
  max_test_case_bytes: number;
  max_files_per_test_case: number;
  retention_days: number;
  allowed_kinds: string[];
  note: string;
}

export function getDatasetLimits() {
  return apiFetch<DatasetLimits>("/test-cases/limits");
}

export function useDatasetLimits() {
  return useQuery({ queryKey: queryKeys.lab.datasetLimits, queryFn: getDatasetLimits });
}

export interface Dataset {
  dataset_id: string;
  file_name: string;
  content_type: string;
  size_bytes: number;
  content_hash: string;
  expires_at: string;
}

export async function uploadDataset(testCaseId: string, file: File) {
  const form = new FormData();
  form.append("file", file);
  // No Content-Type header: the browser sets the multipart boundary itself.
  const result = await apiFetch<unknown>(`/test-cases/${testCaseId}/datasets`, {
    method: "POST",
    body: form,
  });
  if (!isDataset(result)) throw new Error("Invalid dataset response");
  return result;
}

function isDataset(value: unknown): value is Dataset {
  if (!value || typeof value !== "object") return false;
  const dataset = value as Partial<Dataset>;
  return (
    [
      dataset.dataset_id,
      dataset.file_name,
      dataset.content_type,
      dataset.content_hash,
      dataset.expires_at,
    ].every((field) => typeof field === "string" && field.trim().length > 0) &&
    typeof dataset.size_bytes === "number" &&
    Number.isSafeInteger(dataset.size_bytes) &&
    dataset.size_bytes >= 0
  );
}

export function useUploadDataset(testCaseId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (file: File) => uploadDataset(testCaseId, file),
    onSettled: (_result, error) => {
      if (!error || isWriteResultUnknown(error))
        void client.invalidateQueries({ queryKey: queryKeys.testCases.datasets(testCaseId) });
    },
  });
}

export function refusalReason(error: unknown): string | undefined {
  if (!(error instanceof ApiError)) return undefined;
  const reason = (error.body as { reason?: unknown } | undefined)?.reason;
  return typeof reason === "string" ? reason : undefined;
}

export function isRunStartUnknown(error: unknown): boolean {
  if (error instanceof RunNotStartedError) return false;
  if (refusalReason(error) !== undefined) return false;
  return isWriteResultUnknown(error);
}

export class RunNotStartedError extends Error {
  constructor(cause: unknown) {
    super("Run start not sent", { cause });
  }
}

export interface RunCreated {
  run_id: string;
  status: string;
  provider: string;
}

export async function getPreflight(skillId: string, versionId: string, testCaseId: string) {
  const params = new URLSearchParams({ version_id: versionId, test_case_id: testCaseId });
  const result = await apiFetch<PreflightResponse>(
    `/skills/${skillId}/runs/preflight?${params.toString()}`,
  );
  if (
    typeof result?.summary?.skill_version_id !== "string" ||
    typeof result?.summary?.test_case_id !== "string" ||
    result.summary.skill_version_id.toLowerCase() !== versionId.toLowerCase() ||
    result.summary.test_case_id.toLowerCase() !== testCaseId.toLowerCase()
  )
    throw new Error("Invalid preflight response context");
  return result;
}

export function usePreflight(
  skillId: string,
  versionId: string,
  testCaseId: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: queryKeys.lab.preflight(skillId, versionId, testCaseId),
    queryFn: () => getPreflight(skillId, versionId, testCaseId),
    enabled,
    staleTime: 0,
    gcTime: 0,
  });
}

export function confirmPreflight(
  skillId: string,
  versionId: string,
  testCaseId: string,
  summaryHash: string,
) {
  return apiFetch<{ confirmed: boolean; summary_hash: string }>(
    `/skills/${skillId}/runs/preflight/confirm`,
    jsonInit("POST", {
      version_id: versionId,
      test_case_id: testCaseId,
      summary_hash: summaryHash,
    }),
  );
}

export async function startRun(
  skillId: string,
  versionId: string,
  testCaseId: string,
  confirmedSummaryHash: string,
) {
  const result = await apiFetch<unknown>(
    `/skills/${skillId}/runs`,
    jsonInit("POST", {
      version_id: versionId,
      test_case_id: testCaseId,
      confirmed_summary_hash: confirmedSummaryHash,
    }),
  );
  if (!isRunCreated(result)) throw new Error("Invalid run response");
  return result;
}

function isRunCreated(value: unknown): value is RunCreated {
  if (!value || typeof value !== "object") return false;
  const run = value as Partial<RunCreated>;
  return [run.run_id, run.status, run.provider].every(
    (field) => typeof field === "string" && field.trim().length > 0,
  );
}

export function useConfirmAndStartRun(skillId: string, versionId: string, testCaseId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (summaryHash: string) => {
      try {
        await confirmPreflight(skillId, versionId, testCaseId, summaryHash);
      } catch (error) {
        if (isRunStartUnknown(error)) throw new RunNotStartedError(error);
        throw error;
      }
      return startRun(skillId, versionId, testCaseId, summaryHash);
    },
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: queryKeys.runs.lists });
    },
    onError: (err) =>
      err instanceof ApiError && err.status === 422
        ? client.invalidateQueries({
            queryKey: queryKeys.lab.preflight(skillId, versionId, testCaseId),
          })
        : undefined,
  });
}
