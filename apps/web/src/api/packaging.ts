import { useQuery } from "@tanstack/react-query";
import { API_BASE_URL, apiFetch } from "./client";
import type { Finding } from "./types";

/**
 * The PACK-001/002 packaging surface and the WS-002/WS-004 download history,
 * exactly as contracts/openapi/public.yaml declares them (implementation
 * rule 12).
 *
 * Two shapes here are load-bearing rather than stylistic:
 *
 * 1. **The preview is never cached.** `allowed` is computed against the package
 *    as it stands and a licensing hold can be applied while the page is open, so
 *    a preview served from cache would offer a build the server has already
 *    started refusing. Same `staleTime: 0, gcTime: 0` as the suggestion diff,
 *    and for the same reason.
 * 2. **The bytes are a plain link, not a fetch.** The platform streams them from
 *    a session-scoped route (packaging-design §7.1); a top-level GET carries the
 *    Lax session cookie on its own, and `Content-Disposition: attachment` means
 *    the browser saves the file without leaving the page. Reading it into a blob
 *    first would buy nothing and would hold a whole package in memory.
 */

export type PackagingTargetId = "standard" | "claude-code" | "claude-agent-sdk";

export interface PackagingTarget {
  id: PackagingTargetId;
  /**
   * `standard_package` is the Agent Skills spec package with no agent-specific
   * adaptation; `profile` is a versioned set of install settings for one agent.
   */
  kind: "standard_package" | "profile";
  version: string;
  display_name: string;
  /** Absent for the standard package, which makes no claim about any agent's layout. */
  install_location?: string;
  /**
   * `verified` = Skill Hub has actually installed and run a package through this
   * target. `unverified` = it has not, and nothing promises the package works
   * (02:PACK-002 第 2 條). A property of the target, never of a skill.
   */
  support_status: "verified" | "unverified";
  /**
   * A prompt to run against the target agent after installing, to see whether
   * the Skill was picked up. Absent for the standard package, which names no
   * agent to run it against.
   */
  verification_prompt?: string;
  /**
   * Post-install checks, in order (02:PACK-002 第 3 條). Every target has at
   * least one of these two fields; which one depends on whether it names an
   * agent, so both are optional individually.
   */
  verification_steps?: string[];
  /**
   * Environment variables this target needs (02:PACK-002 第 1 條「環境變數需求」).
   * Required by the contract, so an empty array is the answer 「這個目標不需要
   * 環境變數」 and never 「沒有人回答」.
   *
   * `example` is a placeholder and never a credential (iron rule 11): the same
   * value is rendered verbatim into INSTALL.md, which ships inside packages.
   */
  env_vars: PackagingEnvVar[];
  /** Known limitations and the steps a path alone does not cover. Server-owned copy. */
  notes: string[];
}

export interface PackagingEnvVar {
  name: string;
  required: boolean;
  description: string;
  example?: string;
}

export type PackagingBlockedReason =
  "license_hold" | "not_redistributable" | "license_unknown" | "validation_blocked";

export interface PackageValidation {
  /** True when `errors` is non-empty. Stated by the server so no surface decides for itself. */
  blocked: boolean;
  errors: Finding[];
  warnings: Finding[];
  infos: Finding[];
}

export interface IncludedTestCase {
  test_case_id: string;
  name: string;
  /** The directory it gets under `test-cases/` in the zip. Stable across packagings. */
  slug: string;
}

export interface ExcludedTestCase {
  test_case_id: string;
  name: string;
  /** Why it will not travel. Server-owned copy. */
  reason: string;
}

export interface PackagingPreview {
  target: PackagingTargetId;
  allowed: boolean;
  /** Present exactly when `allowed` is false. */
  blocked_reason?: PackagingBlockedReason;
  /** The same sentence the 422 from the packaging call carries. Safe to display as-is. */
  blocked_message?: string;
  validation: PackageValidation;
  /**
   * What the package needs installed (02:PACK-002 第 1 條「依賴」), as the exact
   * lines the produced INSTALL.md carries — same `skillpkg` findings, so the page
   * and the packaged document cannot disagree about what a Skill needs.
   *
   * Empty means two different things and the page must not merge them: a gate
   * closed before any bytes were read (there is no package yet to have
   * dependencies), or the package declares and imports nothing. `allowed` tells
   * them apart.
   */
  dependencies: string[];
  /** Empty when the caller did not ask for them — which is not "there are none". */
  included_test_cases: IncludedTestCase[];
  excluded_test_cases: ExcludedTestCase[];
}

export interface DownloadArtifact {
  artifact_id: string;
  skill_id: string;
  skill_version_id: string;
  target: PackagingTargetId;
  /** Display and attachment name only; never a storage path. */
  file_name: string;
  size_bytes: number;
  /** Over the zip bytes: "is this the file I had". */
  content_hash: string;
  /** Canonical over {path: sha256}: "is the content the same as last time". */
  manifest_hash: string;
  /** `rejected` and `quarantined` are two different things; only `available` is served. */
  status: "quarantined" | "available" | "rejected";
  /** Why it is `rejected`, in the user's language. Absent on the other two states. */
  status_reason?: string;
  /** Absolute date, never "in N days" (PDM-006 risk table). */
  expires_at: string;
  created_at: string;
  /** How many times the bytes were actually served. */
  download_count: number;
  /** Part of the idempotency key: with and without is two artifacts, not one that changed. */
  includes_test_cases: boolean;
  packager_version?: string;
  /** Absent for `standard`, which has no profile. */
  profile_version?: string;
}

export interface CreatedDownloadArtifact extends DownloadArtifact {
  /** True: an identical package already existed and this is it, not a second copy. */
  duplicate: boolean;
}

export function usePackagingTargets() {
  return useQuery({
    queryKey: ["packaging", "targets"],
    queryFn: () => apiFetch<{ targets: PackagingTarget[] }>("/packaging/targets"),
    retry: false,
  });
}

export function usePackagingPreview(
  skillId: string,
  versionId: string,
  target: string,
  includeTestCases: boolean,
) {
  return useQuery({
    queryKey: ["packaging", "preview", skillId, versionId, target, includeTestCases],
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
    retry: false,
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

export function useDownloads() {
  return useQuery({
    queryKey: ["downloads"],
    queryFn: () => apiFetch<{ downloads: DownloadArtifact[] }>("/downloads"),
    retry: false,
  });
}

/** Idempotent by contract: 204 for an id that is not there, which is not a failure. */
export function deleteDownload(artifactId: string) {
  return apiFetch<void>(`/downloads/${artifactId}`, { method: "DELETE" });
}

export function downloadHref(artifactId: string): string {
  return `${API_BASE_URL}/downloads/${artifactId}/content`;
}
