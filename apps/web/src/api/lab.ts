import { apiFetch } from "./client";

/**
 * Test Lab: the pre-run permission gate (02:TEST-005, 03:TEST-008/009).
 *
 * Three calls, in this order and no other: read the summary, agree to the hash
 * that came with it, start the run quoting that hash. The server rebuilds the
 * summary on both of the last two, so a client that skips a step or replays an
 * old hash is refused with 422 — this module cannot grant permission by getting
 * the sequence wrong, it can only fail to ask.
 */

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
  /** `unavailable` is not `none`: an unreadable package is never shown as clean. */
  scripts: { status: "none" | "present" | "unavailable"; findings: string[] };
  tools: string[];
  /** Always empty in the MVP; rendered as an explicit 無, never hidden. */
  mcp_servers: string[];
  network: { mode: string; allow: string[] };
  /** Names of injected secrets. The values never leave the gateway. */
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
    token_budget: { max_input_tokens: number; max_output_tokens: number };
  };
}

/**
 * PDM-005 §5.3/§5.2a-6. A range, not a number: prompt caching makes a first run
 * and a repeat differ ~8x. Deliberately outside `summary`, so it is outside the
 * hash — recalibrating an estimate is not a permission change (see the Go type).
 */
export interface CostEstimate {
  currency: string;
  low: number;
  typical: number;
  high: number;
  basis: string;
}

export interface PreflightResponse {
  summary: PreflightSummary;
  summary_hash: string;
  estimated_cost: CostEstimate;
  notes: string[];
}

/** 02:TEST-002 — the upload rules, shown before an upload rather than on refusal. */
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
}

export function uploadDataset(testCaseId: string, file: File) {
  const form = new FormData();
  form.append("file", file);
  // No Content-Type header: the browser has to set the multipart boundary.
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
