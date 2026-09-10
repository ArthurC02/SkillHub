import { apiFetch } from "./client";

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
  scripts: { status: "none" | "present" | "unavailable"; findings: string[] };
  tools: string[];
  mcp_servers: string[];
  network: { mode: string; allow: string[] };
  injected_secrets: string[];
  provider: {
    name: string;
    isolation_level?: string;
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
  quota?: RunQuota;
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

export interface Dataset {
  dataset_id: string;
  file_name: string;
  content_type: string;
  size_bytes: number;
  content_hash: string;
  expires_at: string;
}

export function uploadDataset(testCaseId: string, file: File) {
  const form = new FormData();
  form.append("file", file);
  // No Content-Type header: the browser sets the multipart boundary itself.
  return apiFetch<Dataset>(`/test-cases/${testCaseId}/datasets`, { method: "POST", body: form });
}

export interface RunCreated {
  run_id: string;
  status: string;
  provider: string;
}

export function getPreflight(skillId: string, versionId: string, testCaseId: string) {
  const params = new URLSearchParams({ version_id: versionId, test_case_id: testCaseId });
  return apiFetch<PreflightResponse>(`/skills/${skillId}/runs/preflight?${params.toString()}`);
}

export function confirmPreflight(
  skillId: string,
  versionId: string,
  testCaseId: string,
  summaryHash: string,
) {
  return apiFetch<{ confirmed: boolean; summary_hash: string }>(
    `/skills/${skillId}/runs/preflight/confirm`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        version_id: versionId,
        test_case_id: testCaseId,
        summary_hash: summaryHash,
      }),
    },
  );
}

export function startRun(
  skillId: string,
  versionId: string,
  testCaseId: string,
  confirmedSummaryHash: string,
) {
  return apiFetch<RunCreated>(`/skills/${skillId}/runs`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      version_id: versionId,
      test_case_id: testCaseId,
      confirmed_summary_hash: confirmedSummaryHash,
    }),
  });
}
