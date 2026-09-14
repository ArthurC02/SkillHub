import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { API_BASE_URL, apiFetch } from "../../core/api/client";
import { queryKeys } from "../../core/api/queryKeys";
import type { Finding, Labelled } from "../../core/api/types";

export type PackagingTargetId = "standard" | "claude-code" | "claude-agent-sdk";

export interface PackagingTarget {
  id: PackagingTargetId;
  kind: "standard_package" | "profile";
  version: string;
  display_name: string;
  install_location?: string;
  support_status: "verified" | "unverified";
  verification_prompt?: string;
  verification_steps?: string[];
  env_vars: PackagingEnvVar[];
  notes: string[];
}

export interface PackagingEnvVar {
  name: string;
  required: boolean;
  description: string;
  example?: string;
}

export type PackagingBlockedReason =
  | "license_hold"
  | "not_redistributable"
  | "license_unknown"
  | "validation_blocked"
  | "file_removed_by_packager";

export interface PackageValidation {
  blocked: boolean;
  errors: Finding[];
  warnings: Finding[];
  infos: Finding[];
}

export interface IncludedTestCase {
  test_case_id: string;
  name: string;
  slug: string;
}

export interface ExcludedTestCase {
  test_case_id: string;
  name: string;
  reason: string;
  label: string;
  note: string;
}

export interface ExcludedFile {
  path: string;
  reason: "excluded_dir" | "credential_file" | "not_a_regular_file" | "unsafe_path";
  label: string;
  note: string;
  referenced_by_skill_md?: boolean;
}

export interface PackagingPreview {
  target: PackagingTargetId;
  allowed: boolean;
  blocked_reason?: PackagingBlockedReason;
  blocked_message?: string;
  validation: PackageValidation;
  dependencies: string[];
  included_test_cases: IncludedTestCase[];
  excluded_test_cases: ExcludedTestCase[];
  excluded_files: ExcludedFile[];
  retention_days: number;
}

export interface DownloadArtifact {
  artifact_id: string;
  skill_id: string;
  skill_version_id: string;
  target: PackagingTargetId;
  file_name: string;
  size_bytes: number;
  content_hash: string;
  manifest_hash: string;
  status: "quarantined" | "available" | "rejected";
  servable: boolean;
  serve_state: Labelled;
  version_number: number;
  latest_version_number: number;
  version_state: Labelled;
  expires_at: string;
  created_at: string;
  download_count: number;
  includes_test_cases: boolean;
  packager_version?: string;
  profile_version?: string;
}

export interface CreatedDownloadArtifact extends DownloadArtifact {
  duplicate: boolean;
}

export function usePackagingTargets() {
  return useQuery({
    queryKey: queryKeys.packaging.targets,
    queryFn: () => apiFetch<{ targets: PackagingTarget[] }>("/packaging/targets"),
  });
}

export function usePackagingPreview(
  skillId: string,
  versionId: string,
  target: string,
  includeTestCases: boolean,
) {
  return useQuery({
    queryKey: queryKeys.packaging.preview(skillId, versionId, target, includeTestCases),
    queryFn: () => {
      const params = new URLSearchParams({
        target,
        include_test_cases: String(includeTestCases),
      });
      return apiFetch<PackagingPreview>(
        `/skills/${skillId}/versions/${versionId}/packaging/preview?${params.toString()}`,
      );
    },
    enabled: skillId !== "" && versionId !== "" && target !== "",
    staleTime: 0,
    gcTime: 0,
  });
}

export function createDownloadArtifact(
  skillId: string,
  versionId: string,
  target: PackagingTargetId,
  includeTestCases: boolean,
) {
  return apiFetch<CreatedDownloadArtifact>(`/skills/${skillId}/versions/${versionId}/packaging`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ target, include_test_cases: includeTestCases }),
  });
}

export type DownloadRequest = {
  versionId: string;
  target: PackagingTargetId;
  includeTestCases: boolean;
};

export function useCreateDownload(skillId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ versionId, target, includeTestCases }: DownloadRequest) =>
      createDownloadArtifact(skillId, versionId, target, includeTestCases),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.packaging.downloads }),
  });
}

export function useDownloads() {
  return useQuery({
    queryKey: queryKeys.packaging.downloads,
    queryFn: () => apiFetch<{ downloads: DownloadArtifact[] }>("/downloads"),
  });
}

export function useRefreshDownloads() {
  const client = useQueryClient();
  return () => void client.invalidateQueries({ queryKey: queryKeys.packaging.downloads });
}

export interface DownloadRecord {
  downloaded_at: string;
  actor: string;
}

export function useDownloadRecords(artifactId: string, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.packaging.downloadRecords(artifactId),
    queryFn: () => apiFetch<{ records: DownloadRecord[] }>(`/downloads/${artifactId}/records`),
    enabled,
  });
}

export function deleteDownload(artifactId: string) {
  return apiFetch<void>(`/downloads/${artifactId}`, { method: "DELETE" });
}

export function useDeleteDownload() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: deleteDownload,
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.packaging.downloads }),
  });
}

export function downloadHref(artifactId: string): string {
  return `${API_BASE_URL}/downloads/${artifactId}/content`;
}
