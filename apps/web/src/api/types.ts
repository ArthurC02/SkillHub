// Hand-written mirror of `components.schemas` in contracts/openapi/public.yaml.
//
// That file is the single source of truth for the contract (implementation
// rule 12). Codegen into packages/api-client-ts is not wired yet (ADR-019 §1),
// so these interfaces are transcribed by hand and carry a standing sync
// obligation: when public.yaml changes, change this file in the same commit.
//
// Optionality here mirrors the schema's `required` list, not what the server
// happens to send today. A field the spec does not require stays `?` even if
// the current handler always fills it — that is the difference between the
// contract and one implementation of it.
//
// Schema name -> interface name is 1:1 unless noted.

export interface Me {
  user_id: string;
  email: string;
  display_name: string;
  workspace_id: string;
}

/**
 * `Labelled`: an enum value together with the copy shown for it. The label and
 * note are server-owned so every surface explains a trust state the same way
 * and stays factual about what was actually checked (NFR-001) — the UI renders
 * them, it does not map the value to its own wording.
 */
export interface Labelled {
  value: string;
  label: string;
  note: string;
}

export type FindingSeverity = "error" | "warning" | "info";

export interface Finding {
  severity: FindingSeverity;
  code: string;
  path?: string;
  message: string;
  /** The full list behind an aggregated finding, when the message summarises. */
  details?: string[];
}

// ---- GET /api/skills/search (DISC-001, DISC-002, DISC-005) ----

export type MatchReasonSource = "model" | "template";

export interface PublicSearchResult {
  skill_id: string;
  name: string;
  summary: string;
  /** Cosine similarity 0..1, higher is better. 0 = lexical-only hit, unranked. */
  rank: number;
  match_reason?: string;
  /** ADR-013: model-generated copy must be labelled as such in the UI. */
  match_reason_source?: MatchReasonSource;
}

export interface PublicSearchResponse {
  query: string;
  results: PublicSearchResult[];
  /** Vector leg did not run; lexical only, so recall is materially lower. */
  degraded: boolean;
  degraded_reason?: string;
  /** At least one result has no embedding yet, so the cut-off never judged it. */
  partial_index: boolean;
  no_results: boolean;
  /** Copy to show with a no-results answer. Absent when there are results. */
  query_suggestion?: string;
}

// ---- GET /api/skills/{id} (DISC-006, DISC-008) ----

export interface SkillSource {
  type: "git" | "upload";
  url?: string;
  source_version?: string;
  fetched_at?: string;
  content_hash?: string;
  /** unknown | traceable | manually_confirmed */
  trust: Labelled;
}

/** ADR-021 provenance tier, strongest first. */
export type LicenseSource = "manifest" | "package-license-file" | "repo-license-file";

export interface SkillLicense {
  /** SPDX id. Absent means unknown, which must never be shown as permissive. */
  expression?: string;
  /** Absent on versions imported before ADR-021; the tier must not be invented. */
  source?: LicenseSource;
  source_note?: string;
  /** unknown | declared. `confirmed` needs a reviewer and is never returned. */
  status: Labelled;
}

export interface SeverityCounts {
  errors: number;
  warnings: number;
  infos: number;
}

export interface SkillRisk {
  /** `unavailable` = the package could not be read; never a clean scan. */
  scan_status: "scanned" | "unavailable";
  counts: SeverityCounts;
  /** Every error- and warning-level finding, verbatim. */
  highlights: Finding[];
  /** Info-level disclosures folded to a count per finding code. */
  info_counts: Record<string, number>;
  has_scripts?: boolean;
  /** Runnable code inside SKILL.md itself (SKILL-003); the file tree cannot show it. */
  has_embedded_script?: boolean;
  has_external_urls?: boolean;
  has_possible_secrets?: boolean;
  has_binaries?: boolean;
  note: string;
}

export type CompatibilityResult = "unverified" | "passed" | "failed";

export interface SkillCompatibility {
  spec_validation: CompatibilityResult;
  capability: CompatibilityResult;
  runtime: CompatibilityResult;
  note: string;
}

/** Index-time model output (ADR-013 §1), labelled as model-written. */
export interface SkillEnrichment {
  status: "pending" | "enriched";
  summary?: string;
  task_examples?: string[];
  tags?: string[];
  model?: string;
  prompt_version?: string;
  note: string;
}

/** The inline `version` object on SkillDetail. */
export interface SkillVersionSummary {
  version_id: string;
  version_number: number;
  content_hash: string;
  created_at: string;
}

/** The inline `derivation` object on SkillDetail (DISC-003, WS-001). */
export interface SkillDerivation {
  is_fork: boolean;
  forked_from_skill_id?: string;
  forked_from_version_id?: string;
  label: string;
  note: string;
}

export interface SkillDetail {
  skill_id: string;
  name: string;
  /** The package's own frontmatter description, never the model's rewrite. */
  summary: string;
  scope: "catalog" | "private";
  /** curated | indexed | external. Always `indexed` today (PDM-002). */
  tier: Labelled;
  enrichment: SkillEnrichment;
  /** Absent for a skill with no saved content yet. */
  version?: SkillVersionSummary;
  source?: SkillSource;
  license: SkillLicense;
  derivation: SkillDerivation;
  allowed_tools?: string[];
  risk: SkillRisk;
  compatibility: SkillCompatibility;
}

// ---- GET /api/skills/{id}/files (DISC-007) ----

export interface SkillFileEntry {
  path: string;
  /** Uncompressed size in bytes. */
  size: number;
  is_script: boolean;
}

export interface SkillFiles {
  skill_id: string;
  version_id: string;
  version_number: number;
  skill_md: string;
  skill_md_truncated: boolean;
  /** Every file, sorted by path. Directories are omitted; the paths carry them. */
  tree: SkillFileEntry[];
  /** SKILL-003 disclosure: this tree cannot show code living inside SKILL.md. */
  embedded_script_note?: string;
  note: string;
}

// ---- POST /skills/{id}/fork (WS-001) ----

export interface Skill {
  skill_id: string;
  name: string;
  summary: string;
  forked_from_skill_id?: string;
  forked_from_version_id?: string;
}

export interface ForkedSkill extends Skill {
  version_id: string;
  version_number: number;
}
