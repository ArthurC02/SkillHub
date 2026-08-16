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

/**
 * `SearchResultRisk`: compact risk hint for a result row (DISC-002 風險提示).
 * Read from the search projection rather than a fresh scan, so there are no
 * verbatim findings here — the detail view has those. No "safe" level exists
 * (NFR-001).
 */
export interface SearchResultRisk {
  /** `unavailable` = the projection holds no scan; never a clean scan. */
  scan_status: "scanned" | "unavailable";
  /** Highest severity recorded. Errors block import, so they never appear. */
  level: "none" | "disclosed" | "warning";
  warnings: number;
  has_scripts?: boolean;
  has_embedded_script?: boolean;
  has_external_urls?: boolean;
  has_possible_secrets?: boolean;
  has_binaries?: boolean;
  has_dependency_manifest?: boolean;
  note: string;
}

export interface PublicSearchResult {
  skill_id: string;
  name: string;
  summary: string;
  /**
   * Cosine similarity 0..1, higher is better; the array order follows it.
   *
   * Null when the page was not ranked by similarity — the whole answer came
   * from the lexical leg (`degraded`), or this row has no embedding yet
   * (`partial_index`). The unbounded lexical score is not returned in its
   * place; `rank_note` says what ordered the page instead.
   */
  rank: number | null;
  /** Why `rank` is null. Present only when it is. */
  rank_note?: string;
  /** 來源層級. Always `indexed` today (PDM-002). */
  tier: Labelled;
  risk: SearchResultRisk;
  /** Dependency tags from enrichment; empty while it is pending. */
  dependencies: string[];
  /** Same three axes as the detail view; capability/runtime are 尚未試跑. */
  compatibility: SkillCompatibility;
  /** Latest version's creation time — the import that scanned it. */
  verified_at?: string;
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
  /**
   * The query matched skills and the active filters removed all of them.
   * Never true at the same time as `no_results`: one asks the user to widen a
   * filter, the other to describe the task differently, so the copy for the two
   * empty states must not be shared.
   */
  filtered_out: boolean;
  /** Copy to show with a no-results answer. Absent when there are results. */
  query_suggestion?: string;
}

/**
 * DISC-003 filter state, mirrored 1:1 by the URL search params so a filtered
 * result page is linkable. `undefined` is "this dimension is not filtered",
 * which is a third state distinct from `no`/`unverified`.
 *
 * Only the two dimensions with per-row data are here. 類別／來源層級／Agent
 * 相容／MCP are rejected by the server with the reason why (see
 * UNAVAILABLE_FILTERS in Home.tsx) and are rendered as disabled controls.
 */
export interface SearchFilters {
  script?: "yes" | "no";
  validation?: "passed" | "unverified";
  /** DISC-002 Agent dimension, runtime axis (02:DISC-002 篩選維度的允收階段: M2). */
  agent?: AgentRuntime;
}

// ---- GET /api/skills/{id} (DISC-006, DISC-008) ----

export interface SkillSource {
  type: "git" | "upload";
  url?: string;
  source_version?: string;
  fetched_at?: string;
  content_hash?: string;
  /**
   * When the upstream-availability probe last looked. Absent means never
   * probed, which is not the same as "available" and must not be shown as one.
   */
  last_checked_at?: string;
  /**
   * When the source *started* failing, not the latest failure, so a two-week
   * outage stays distinguishable from a blip. Absent means it answered last time.
   */
  unavailable_since?: string;
  /** unknown | traceable | manually_confirmed */
  trust: Labelled;
}

/** ADR-021 provenance tier, strongest first. */
export type LicenseSource =
  "manifest" | "manifest-referenced-file" | "package-license-file" | "repo-license-file";

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

/** Did the agent pick the skill up when it was mounted (0022). */
export type AgentCapability = "activated" | "not_activated" | "unverified";

/**
 * Could the package's own scripts run in the runtime image it was measured on
 * (0022). `transpiled` is the answer the M2 baseline actually produced for 33 of
 * 45 skills: the Run worked, but what ran was the model's re-implementation of
 * the script rather than the script, because the image had no interpreter for it.
 */
export type AgentRuntime = "native" | "transpiled" | "failed" | "unverified";

export interface SkillCompatibility {
  spec_validation: CompatibilityResult;
  capability: AgentCapability;
  runtime: AgentRuntime;
  /**
   * The runtime image the two axes above were measured on. Absent = never
   * measured, which is also when both axes read `unverified`. It is not a
   * detail: the same skill answers differently on an image with a different set
   * of interpreters, so a verdict shown without it is a claim nobody made.
   */
  runtime_image?: string;
  measured_at?: string;
  note: string;
}

/**
 * Input/output/tool/dependency tags in their buckets. DISC-003 一般模式 asks
 * for 輸入、輸出、依賴 separately and a flat list cannot say which is which.
 */
export interface SkillTags {
  inputs: string[];
  outputs: string[];
  tools: string[];
  dependencies: string[];
}

/** Index-time model output (ADR-013 §1), labelled as model-written. */
export interface SkillEnrichment {
  status: "pending" | "enriched";
  summary?: string;
  task_examples?: string[];
  tags?: SkillTags;
  model?: string;
  prompt_version?: string;
  note: string;
}

/**
 * One stated limitation (DISC-003「限制」) with its provenance. `model` is the
 * enrichment restating what the document says about its own limits; `scan` is
 * derived from the static package scan. ADR-013 requires the model half to be
 * labelled, so the two are never merged into one anonymous sentence.
 */
export interface SkillLimitation {
  text: string;
  source: "model" | "scan";
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
  /**
   * 限制 (DISC-003), from both the enrichment and the scan, each labelled.
   * Empty means neither source stated one — never that the skill has no limits.
   */
  limitations: SkillLimitation[];
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
