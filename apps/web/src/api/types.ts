import type { CategorizedFindings, ImportResult } from "./import";

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
  /**
   * When the caller asked for their account to be deleted, `null` when nothing
   * is pending (02:SEC-006「刪除工作具可追蹤狀態」). Required by the schema and
   * therefore not optional here: `null` and "the field is missing" would be the
   * difference between 「沒有申請」 and 「申請了但我看不到」, and only the first is a
   * state the product has.
   */
  deletion_requested_at: string | null;
  /**
   * The server's own sentence about what the purge destroys and what it keeps
   * de-identified, `null` when nothing is pending. It is the same wording
   * DELETE /me returns, and it is here because a disclosure that only existed in
   * that one response survived exactly one render: a reload lost it while the
   * grace period it described ran on (04 丙-30).
   */
  deletion_scope: string | null;
  /**
   * Optional entry points this deployment turns on (ADR-052). **Absent** when
   * there are none — not an empty object, so there is never a difference to
   * resolve between "off" and "this build predates the flag".
   *
   * It is here because a route that is simply not mounted cannot be discovered
   * without a request that fails, and a feature discovered by a failed request
   * has already been drawn on somebody's screen.
   */
  features?: Record<string, boolean>;
  /** When the grace period ends. Null exactly when `deletion_requested_at` is. */
  purge_after: string | null;
}

/**
 * The answer to DELETE /me. `scope` is a server-owned plain-language statement
 * of what goes and what is kept de-identified — rendered verbatim, never
 * paraphrased on this side: a second copy of it in the client is a second thing
 * to keep true (WS-002 要求刪除前先說明範圍).
 */
export interface AccountDeletion {
  deletion_requested_at: string;
  purge_after: string;
  cancellable: boolean;
  scope: string;
}

/** The answer to DELETE /skills/{id} (WS-005); `note` states the scope. */
export interface SkillDeletion {
  deleted: boolean;
  versions_retained: number;
  note: string;
}

// ---- GET /policy/data-retention (02:O11Y-004, ADR-029) ----

export interface AnalyticsEventDisclosure {
  name: "search_performed" | "skill_detail_viewed" | "session_started" | "download_started";
  when: string;
  /** The attribute whitelist, one entry per column the event writes. */
  attributes: string[];
  not_recorded: string;
}

export interface DataRetentionPolicy {
  /** False is the shipped default: no row written, no cookie set. */
  collecting: boolean;
  /** Zero exactly when `collecting` is false. Never a number the UI invents. */
  retention_days: number;
  events: AnalyticsEventDisclosure[];
  note: string;
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

/**
 * skills.redistribution — the licence lock on downloads (0027, extended by 0036
 * and 0037). Named rather than inlined so the places that decide something from
 * it can be keyed by it: a `Record<Redistribution, ...>` stops compiling when a
 * value is added here, which is how the web finds out.
 *
 * It found out the hard way once. `generated` arrived in 0037, the server
 * released the gate for it (delivery/packaging.go gateFlags) and this side's
 * `switch` sent it to `default` — so the platform built the package and the UI
 * told the owner nobody had established whether it could be redistributed.
 *
 * `SkillDetail.redistribution` is a `Labelled` whose `value` stays `string`:
 * that one is rendered from the server's own sentences and has nothing to keep
 * in step.
 */
export type Redistribution = "allowed" | "blocked" | "unknown" | "self_supplied" | "generated";

export type FindingSeverity = "error" | "warning" | "info";

export interface Finding {
  severity: FindingSeverity;
  code: string;
  path?: string;
  message: string;
  /** The full list behind an aggregated finding, when the message summarises. */
  details?: string[];
}

/**
 * `Disclosure`: one thing a package declares about itself, with the words for it.
 *
 * It replaced the parallel `has_*` booleans on both risk shapes (04 丙-29 ④), and
 * the reason is what the booleans made easy: the two payloads carried
 * **different sets** — the search row had `has_dependency_manifest` and the
 * detail view did not — so the screen a reader meets first disclosed more than
 * the screen they open next. A list has no shape to disagree on.
 *
 * `code` is not a union. A code this build has never heard of still arrives with
 * a label and a note, and narrowing the type would be a way to *hide* a
 * disclosure to keep a union tidy.
 */
export interface Disclosure {
  code: string;
  label: string;
  note: string;
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
  /** What the scan found declared, server-worded. Empty is not 「安全」. */
  disclosures: Disclosure[];
  note: string;
}

export interface PublicSearchResult {
  skill_id: string;
  name: string;
  summary: string;
  /**
   * Who wrote `summary` (ADR-013). `package` is also the `description` an agent
   * reads when it decides whether to load the Skill; a `model` summary is not
   * that text, and the downloaded package never carries it.
   */
  summary_source: "model" | "package";
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
  /** How many results this page carries at most; 0 when nothing was retrieved. */
  limit: number;
  /**
   * The catalogue held more matches than this page shows. Separate from
   * `degraded` and `partial_index`: those say how well the search could look,
   * this says how much of what it found is here (ADR-042 決策 3).
   */
  truncated: boolean;
  /**
   * How many matched, before `limit` cut the page down — 設計系統 §4.3's 「共 N
   * 筆」. The page could previously only say 「超過 N 個」, and a lower bound
   * cannot distinguish 21 from 2100. Equals `results.length` when `truncated`
   * is false; computed by the retrieval statement itself, so it cannot drift
   * from the rows it counts.
   */
  total: number;
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
  type: "git" | "upload" | "generated";
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
  /**
   * The user's own words that produced a generated package (GEN-002). Present
   * only for `generated`, and it is that package's ENTIRE provenance record —
   * which is why a generated source is never shown as unknown. It is known; it
   * just is not a URL.
   */
  task_description?: string;
  generator_model?: string;
  generator_prompt_version?: string;
  /**
   * unknown | traceable | manually_confirmed | generated. `generated` is not a
   * rung above `unknown` on the same ladder: there is nothing upstream to trace
   * to, and it claims nothing about quality or safety.
   */
  trust: Labelled;
}

/**
 * The 422 from POST /skills/generate: the findings a failed import would return,
 * plus how many attempts produced them.
 *
 * `attempts` is on the failure and not only on the success because the failure
 * screen has a sentence about the automatic retry, and that sentence is false
 * for a `possible-secret` finding — ADR-048 does not retry that one.
 */
export interface GenerateRejected extends CategorizedFindings {
  attempts: number;
}

/**
 * POST /skills/generate — ImportResult plus how it got here (GEN-001).
 *
 * It extends the import shape rather than restating it: a generated package IS
 * an import on the server side, through the same validation path and the same
 * version write, and two descriptions of one row is how the two screens start
 * disagreeing about what a version is.
 */
/**
 * One generation that produced nothing — GET /skills/generate/failures.
 *
 * 02:GEN-003 「在工作區留下可查的失敗紀錄」. Every field but `occurred_at` is
 * best-effort: these rows are 400-day history written by whichever version of
 * the code was running at the time, so a missing key produces a row with less in
 * it rather than an error.
 *
 * **The task description is not here and must not be added.** It belongs to the
 * source row, under NFR-002 deletion; audit rows live 400 days under a different
 * rule, and one copy under each is a retention promise nobody made.
 */
export interface GenerationFailure {
  occurred_at: string;
  /** Empty when the row's metadata could not be decoded. The row still happened. */
  failure: "quota" | "unavailable" | "gateway" | "unpackageable" | "rejected" | "blocked" | "";
  /** 0 for a refusal that never reached the gateway — `quota` and `unavailable`. */
  attempts: number;
  /** Blocking finding codes, `blocked` only. Codes and never matched values. */
  codes?: string[];
  truncated?: boolean;
  collision?: boolean;
}

export interface GenerateSkillResult extends ImportResult {
  /**
   * 1 or 2. Shown rather than hidden because 02:GEN-003 forbids the UI
   * promising a success rate: one retry was measured to move 80% to 90%, not to
   * nothing, and "it took two goes" is the honest form of the same information.
   */
  attempts: number;
  generator_model: string;
  generator_prompt_version: string;
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
  /**
   * The same catalogue the search row carries — including `dependency-file`,
   * which this shape used to be missing while the row above it showed it.
   */
  disclosures: Disclosure[];
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

/**
 * The three axes arrive labelled (04 丙-29 ③). `value` is still the enum above;
 * the words are the server's, because two screens had already worded this block
 * differently and one of them wrote `passed ? 通過 : 未驗證` — reporting **failed
 * as 未驗證**. `note` is empty for the states the block-level `note` already
 * covers and non-empty exactly where the label alone would be misread.
 */
export interface SkillCompatibility {
  spec_validation: Labelled;
  capability: Labelled;
  runtime: Labelled;
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

/**
 * The answer to GET /skills/{id}/versions (WS-001), newest first. Same shape as
 * the inline `version` above, because it is the same fact about a different
 * number of versions.
 *
 * Workspace scoped server-side: a skill the caller does not own answers with an
 * empty list, which the screens render as 「沒有可選的版本」 and never as 「這個
 * Skill 沒有版本」.
 */
export interface SkillVersions {
  versions: SkillVersionSummary[];
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
  /**
   * allowed | blocked | unknown — whether this content may be handed on to
   * someone else, which decides whether a download package can be built from it
   * at all (02:SEC-007, ADR-027 決策 4). Only `allowed` releases; `unknown` is
   * where a skill starts and is treated exactly like `blocked`.
   *
   * A separate axis from `license.status` and never derived from it
   * (02:CONTENT-002), and separate from `access_restriction`, which is a
   * reviewer's temporary hold rather than a property of the content.
   *
   * Required, and for every skill: a skill nobody classified is `unknown`, so
   * there is no "no answer" state for this field to be absent for.
   */
  redistribution: Labelled;
  derivation: SkillDerivation;
  allowed_tools?: string[];
  risk: SkillRisk;
  compatibility: SkillCompatibility;
  /**
   * A licensing hold on the package's own materials (migration 0023). Present
   * only while a review is open; the skill stays listed, keeps its summary and
   * its provenance, and only the full text, the file tree and running it are
   * closed. Absent is the normal case.
   */
  access_restriction?: SkillAccessRestriction;
}

export interface SkillAccessRestriction {
  /** Reason code, e.g. `license-review`. */
  reason: string;
  /** The reader-facing explanation, written by the API. */
  note: string;
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
  /**
   * Whether a Download Artifact may be produced from this skill. Three of the
   * five values release and two refuse, so on the owner's own list this
   * separates a skill they can take away from one they cannot. It was on the
   * row and dropped in serialisation until 04 丙-31.
   *
   * `generated` is what the platform's own output carries since 0037. It
   * releases the download for the same shape of reason as `self_supplied` —
   * no upstream author — and is a separate value because the open question
   * differs: who owns what a model wrote (ADR-047 決策 4).
   *
   * `self_supplied` is what a user's own import carries since 0036; it was
   * `unknown`, which refused (ADR-045).
   */
  redistribution: Redistribution;
  /** Reason code for a licensing hold, `null` when there is none. */
  access_restriction: string | null;
  forked_from_skill_id?: string;
  forked_from_version_id?: string;
}

/**
 * GET /skills. `truncated` is the cap saying so: the server has always returned
 * at most 100 rows, and skill 101 simply did not appear — a limit the platform
 * enforces and the page cannot see is 設計 §2.2 in its second direction, and a
 * list that is quietly short reads as a complete answer.
 */
export interface OwnSkills {
  skills: OwnSkill[];
  limit: number;
  truncated: boolean;
  /**
   * How many the workspace holds, before the cap — 設計系統 §4.3's 「共 N 筆」,
   * added 2026-08-25. `truncated` gave the reason all along; this is the count,
   * and 「超過 100 個」 was a lower bound that could not distinguish 101 from
   * 10100. Exact: the listing statement counts its own rows.
   */
  total: number;
}

/**
 * A row of the caller's own list: `Skill` plus the two facets that make the list
 * decidable rather than merely enumerable (設計 §1.1). Not on `Skill` itself —
 * the fork reply shares that shape and a one-second-old fork has neither.
 *
 * `risk` is deliberately the *same* `SearchResultRisk` a search row carries for
 * the same skill, not a second type shaped like it.
 */
export interface OwnSkill extends Skill {
  risk: SearchResultRisk;
  verification: SkillVerification;
}

/**
 * Was anything measured **in this workspace**, and when.
 *
 * `not_measured` is the fork case: the bytes were scanned where it was forked
 * from, and the platform did not re-run anything here. It is not a claim that
 * the content is unsafe — and not a claim that it is fine. `scanned_at` exists
 * only in the `scanned` state, which is the point of naming the state at all:
 * a fork's version row is created the moment somebody presses Fork, so the
 * timestamp that reads as "just scanned" is exactly the one nothing scanned.
 */
export interface SkillVerification extends Labelled {
  value: "scanned" | "not_measured" | "not_applicable";
  scanned_at?: string | null;
}

export interface ForkedSkill extends Skill {
  version_id: string;
  version_number: number;
}
