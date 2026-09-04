import { useQuery } from "@tanstack/react-query";
import { API_BASE_URL, apiFetch } from "./client";
import type { Finding, Labelled } from "./types";

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
  | "license_hold"
  | "not_redistributable"
  | "license_unknown"
  | "validation_blocked"
  /** The one refusal the platform caused itself: SKILL.md points at a file the exporter removed. */
  | "file_removed_by_packager";

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
  /** Machine code, e.g. `not_curated`. Never the visible word — see `label`. */
  reason: string;
  /** The server's word for the reason (設計系統 §4.4). Mirrors `ExcludedFile.label`. */
  label: string;
  /** Why, in the reader's own terms. Mirrors `ExcludedFile.note`. */
  note: string;
}

/**
 * One source file the exporter removed from the author's own tree, and why
 * (ADR-044, `contracts/packaging/download-manifest.schema.json`).
 *
 * On the preview as well as in the download manifest on purpose: the manifest
 * is inside the thing the reader has not decided to download yet, so answering
 * only there is not answering the decision.
 *
 * `label` and `note` are the server's words for the reason (設計系統 §4.4) —
 * every note says how to fix it rather than restating the rule, because the
 * reader is the author who is now missing a file. Never re-worded here: two
 * surfaces wording one removal differently is how a reader ends up believing the
 * platform did two different things.
 */
export interface ExcludedFile {
  /** Package-relative path of the file that was removed. */
  path: string;
  reason: "excluded_dir" | "credential_file" | "not_a_regular_file" | "unsafe_path";
  label: string;
  note: string;
  /**
   * True when SKILL.md points at this path — the one combination the platform
   * treats as its own fault rather than the author's, and the one that turns the
   * whole preview into `file_removed_by_packager`. A reference that was already
   * dangling at import is a different fact and stays a warning.
   */
  referenced_by_skill_md?: boolean;
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
  /**
   * Files the packager removed — `.git/`, `node_modules/`, `.env`, symlinks.
   * Required by the contract, so empty is the answer 「什麼都沒被拿掉」 and never
   * 「沒有人回答」. Empty for almost every package; not empty for the one that
   * matters, a Skill that vendored its dependencies or one whose SKILL.md points
   * at something the exporter would not carry.
   */
  excluded_files: ExcludedFile[];
  /**
   * 03:PACK-011 — how long this package would be kept, in whole days. Served so
   * the page never carries its own copy of a deployment number (設計 §2.2
   * 顯示與強制成對).
   *
   * Required: a deployment with no ratified `DOWNLOAD_ARTIFACT_RETENTION`
   * answers 503 for the whole preview rather than serving one with an unknown
   * period, so there is no state in which this is absent and the page still has
   * something to draw.
   */
  retention_days: number;
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
  /**
   * Whether the content endpoint would hand the bytes over **right now**:
   * `available` AND not purged AND not expired (04 丙-29 ⑤).
   *
   * Served, not derived here, because one of its three inputs — the purge — is
   * not on this shape at all. Every client that derived a word was therefore
   * deriving a different predicate from the one the server enforces, which is
   * 設計 §2.2's 顯示但不強制.
   */
  servable: boolean;
  /**
   * The one sentence for the composite above. `value` is `status` except when
   * an `available` artifact has expired, been purged, or been found missing by
   * the object reconciler, which is where `expired`, `purged` and `lost` come
   * from - values that do not appear in `status` at all, which is why a label on
   * `status` alone would have fought the word on the screen instead of settling
   * it.
   *
   * `lost` is not a shade of `expired` (04 丙-91): expiry is the retention
   * promise being kept, loss is the platform dropping the bytes inside it. The
   * server picks the word AND writes the sentence for it; a surface that prints
   * its own retention copy beside `lost` tells the owner a failure was policy,
   * and the one person who could have reported it stops having a reason to.
   */
  serve_state: Labelled;
  /** Which version these bytes are (04 丙-42). The uuid identifies the row; this identifies the content. */
  version_number: number;
  /** The skill's highest version number right now. Equal to `version_number` when this is the newest. */
  latest_version_number: number;
  /**
   * `current` or `superseded`, wording and numbers from the server.
   *
   * A separate axis from `serve_state` on purpose: a superseded package is still
   * downloadable, and wanting exactly the version you packaged is legitimate.
   * `serve_state` answers 「拿不拿得到位元組」, this answers 「是不是最新的內容」.
   */
  version_state: Labelled;
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

/**
 * GET /downloads/{artifactId}/records — 「誰、何時」, one row per download
 * (WS-004). The list above answers 「哪一筆 artifact、哪一個 profile」 plus a
 * count, and a count cannot answer this.
 *
 * `actor` is a display name and not a user id: on a personal workspace it is
 * always the owner, and an id would be an identifier the reader cannot resolve.
 * It reads 「deleted user」 when the account was purged — the row survives
 * de-identified, because 「somebody, at this time」 is still true.
 *
 * Not the audit trail: the same download writes an audit event too, and the two
 * differ in retention and visibility (03:CORE-008 forbids merging them).
 */
export interface DownloadRecord {
  downloaded_at: string;
  actor: string;
}

export function useDownloadRecords(artifactId: string, enabled: boolean) {
  return useQuery({
    queryKey: ["downloads", artifactId, "records"],
    queryFn: () => apiFetch<{ records: DownloadRecord[] }>(`/downloads/${artifactId}/records`),
    enabled,
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
