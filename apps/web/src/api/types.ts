// Types mirrored by hand from contracts/openapi/public.yaml `components.schemas`.
// Keep these in sync manually until codegen is wired (ADR-019 §1: the real
// client belongs in packages/api-client-ts and must never be hand-edited —
// see api/skills.ts for which endpoints below are not in the contract yet).

export interface Me {
  user_id: string;
  email: string;
  display_name: string;
  workspace_id: string;
}

export interface SearchHit {
  skill_id: string;
  name: string;
  summary: string;
  rank: number;
}

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

// ---- Below: no backend schema yet. Shaped from 02-specifications-and-
// acceptance-criteria.md §3 (quality/trust states) and §4.1 DISC-003. ----

export type SourceTrustLevel = "unknown" | "traceable" | "confirmed";
export type LicenseState = "unknown" | "declared" | "confirmed";
export type CompatibilityResult = "unverified" | "passed" | "failed";
export type VerificationState = "not_run" | "success" | "failure" | "partial";

export interface SkillSource {
  trust_level: SourceTrustLevel;
  url?: string;
  source_version?: string;
  fetched_at?: string;
  content_hash?: string;
}

export interface SkillLicense {
  status: LicenseState;
  name?: string;
}

export interface CompatibilityEntry {
  agent: string;
  runtime?: string;
  status: CompatibilityResult;
}

export interface SkillRisk {
  has_scripts: boolean;
  has_external_urls: boolean;
  has_possible_secrets: boolean;
}

export interface SkillVerification {
  status: VerificationState;
  last_verified_at?: string;
}

export interface SkillDetail extends Skill {
  capabilities: string[];
  limitations: string[];
  inputs: string[];
  outputs: string[];
  dependencies: string[];
  required_permissions: string[];
  source: SkillSource;
  license: SkillLicense;
  compatibility: CompatibilityEntry[];
  verification: SkillVerification;
  risk: SkillRisk;
}

export interface SkillFileEntry {
  path: string;
  type: "file" | "dir";
  is_script: boolean;
}

export interface SkillFiles {
  skill_md: string;
  tree: SkillFileEntry[];
}

export interface SkillVersionSummary {
  version_id: string;
  version_number: number;
  content_hash: string;
  created_at: string;
}
