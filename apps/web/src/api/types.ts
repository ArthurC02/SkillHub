import type { CategorizedFindings, ImportResult } from "./import";

export interface Me {
  user_id: string;
  email: string;
  display_name: string;
  workspace_id: string;
  deletion_requested_at: string | null;
  deletion_scope: string | null;
  features?: Record<string, boolean>;
  purge_after: string | null;
}

export interface AccountDeletion {
  deletion_requested_at: string;
  purge_after: string;
  cancellable: boolean;
  scope: string;
}

export interface SkillDeletion {
  deleted: boolean;
  versions_retained: number;
  note: string;
}

export interface AnalyticsEventDisclosure {
  name: "search_performed" | "skill_detail_viewed" | "session_started" | "download_started";
  when: string;
  attributes: string[];
  not_recorded: string;
}

export interface DataRetentionPolicy {
  collecting: boolean;
  retention_days: number;
  events: AnalyticsEventDisclosure[];
  note: string;
  feedback: DataRetentionPolicyFeedback;
}

export interface DataRetentionPolicyFeedback {
  what: string;
  collected: string[];
  free_text: string;
  kind: ("blocking_issue" | "need_signal")[];
  page_path: string;
  run_id: string;
  on_account_deletion: string;
  retention_days: number | null;
  note?: string;
}

export interface Labelled {
  value: string;
  label: string;
  note: string;
}

export type Redistribution = "allowed" | "blocked" | "unknown" | "self_supplied" | "generated";

export type FindingSeverity = "error" | "warning" | "info";

export interface Finding {
  severity: FindingSeverity;
  code: string;
  path?: string;
  message: string;
  details?: string[];
}

export interface Disclosure {
  code: string;
  label: string;
  note: string;
}

export type MatchReasonSource = "model" | "template";

export interface SearchResultRisk {
  scan_status: "scanned" | "unavailable";
  level: "none" | "disclosed" | "warning" | "unknown";
  warnings: number;
  disclosures: Disclosure[];
  note: string;
}

export interface PublicSearchResult {
  skill_id: string;
  name: string;
  summary: string;
  summary_source: "model" | "package";
  rank: number | null;
  rank_note?: string;
  tier: Labelled;
  category: Labelled;
  risk: SearchResultRisk;
  dependencies: string[];
  compatibility: SkillCompatibility;
  verified_at?: string;
  match_reason?: string;
  match_reason_source?: MatchReasonSource;
}

export interface CatalogResponse {
  results: PublicSearchResult[];
  limit: number;
  total: number;
  truncated: boolean;
}

export interface PublicSearchResponse {
  query: string;
  results: PublicSearchResult[];
  degraded: boolean;
  degraded_reason?: string;
  partial_index: boolean;
  limit: number;
  truncated: boolean;
  total: number;
  no_results: boolean;
  filtered_out: boolean;
  query_suggestion?: string;
}

export interface SearchFilters {
  script?: "yes" | "no";
  validation?: "passed" | "unverified";
  agent?: AgentRuntime;
  tier?: "curated" | "indexed";
  category?: SkillCategory;
}

export type SkillCategory = "documents" | "writing" | "data";

export interface SkillSource {
  type: "git" | "upload" | "generated";
  url?: string;
  source_version?: string;
  fetched_at?: string;
  content_hash?: string;
  last_checked_at?: string;
  unavailable_since?: string;
  task_description?: string;
  generator_model?: string;
  generator_prompt_version?: string;
  generation_inputs?: GenerationInputs;
  trust: Labelled;
}

export interface GenerationInputs {
  diagram?: {
    media_type: "image/png" | "image/jpeg" | "image/webp";
    sha256: string;
    bytes: number;
  };
  references?: { skill_id: string; version_id: string; name: string }[];
}

export interface GenerateRejected extends CategorizedFindings {
  attempts: number;
}

export interface GenerateDiagram {
  media_type: "image/png" | "image/jpeg" | "image/webp";
  data: string;
}

export interface GenerateSkillRequest {
  task_description?: string;
  diagram?: GenerateDiagram;
  reference_skill_ids?: string[];
}

export interface GenerationFailure {
  occurred_at: string;
  failure:
    "quota" | "unavailable" | "gateway" | "unpackageable" | "rejected" | "blocked" | "credit" | "";
  attempts: number;
  codes?: string[];
  truncated?: boolean;
  collision?: boolean;
}

export interface GenerateSkillResult extends ImportResult {
  attempts: number;
  generator_model: string;
  generator_prompt_version: string;
}

export type LicenseSource =
  "manifest" | "manifest-referenced-file" | "package-license-file" | "repo-license-file";

export interface SkillLicense {
  expression?: string;
  source?: LicenseSource;
  source_note?: string;
  status: Labelled;
}

export interface SeverityCounts {
  errors: number;
  warnings: number;
  infos: number;
}

export interface SkillRisk {
  scan_status: "scanned" | "unavailable";
  counts: SeverityCounts;
  highlights: Finding[];
  info_counts: Record<string, number>;
  disclosures: Disclosure[];
  note: string;
}

export type CompatibilityResult = "unverified" | "passed" | "failed";

export type AgentCapability = "activated" | "not_activated" | "unverified";

export type AgentRuntime = "native" | "transpiled" | "failed" | "unverified";

export interface SkillCompatibility {
  spec_validation: Labelled;
  capability: Labelled;
  runtime: Labelled;
  runtime_image?: string;
  measured_at?: string;
  note: string;
}

export interface SkillTags {
  inputs: string[];
  outputs: string[];
  tools: string[];
  dependencies: string[];
}

export interface SkillEnrichment {
  status: "pending" | "enriched";
  summary?: string;
  task_examples?: string[];
  tags?: SkillTags;
  model?: string;
  prompt_version?: string;
  note: string;
}

export interface SkillLimitation {
  text: string;
  source: "model" | "scan";
}

export interface SkillVersionSummary {
  version_id: string;
  version_number: number;
  content_hash: string;
  created_at: string;
}

export interface SkillVersions {
  versions: SkillVersionSummary[];
}

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
  summary: string;
  scope: "catalog" | "private";
  tier: Labelled;
  category: Labelled;
  enrichment: SkillEnrichment;
  limitations: SkillLimitation[];
  version?: SkillVersionSummary;
  source?: SkillSource;
  license: SkillLicense;
  redistribution: Labelled;
  derivation: SkillDerivation;
  allowed_tools?: string[];
  risk: SkillRisk;
  compatibility: SkillCompatibility;
  access_restriction?: SkillAccessRestriction;
}

export interface SkillAccessRestriction {
  reason: string;
  note: string;
}

export interface SkillFileEntry {
  path: string;
  size: number;
  is_script: boolean;
}

export interface SkillFiles {
  skill_id: string;
  version_id: string;
  version_number: number;
  skill_md: string;
  skill_md_truncated: boolean;
  tree: SkillFileEntry[];
  embedded_script_note?: string;
  note: string;
}

export interface Skill {
  skill_id: string;
  name: string;
  summary: string;
  redistribution: Redistribution;
  access_restriction?: string | null;
  forked_from_skill_id?: string;
  forked_from_version_id?: string;
}

export interface OwnSkills {
  skills: OwnSkill[];
  limit: number;
  truncated: boolean;
  total: number;
}

export interface OwnSkill extends Skill {
  risk: SearchResultRisk;
  verification: SkillVerification;
}

export interface SkillVerification extends Labelled {
  value: "scanned" | "not_measured" | "not_applicable";
  scanned_at?: string | null;
}

export interface ForkedSkill extends Skill {
  version_id: string;
  version_number: number;
}
