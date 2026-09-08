import { apiFetch } from "./client";
import { useMe } from "./me";
export type CreationState =
  | "queued"
  | "working"
  | "waiting_input"
  | "waiting_confirmation"
  | "draft_ready"
  | "candidate_ready"
  | "saved"
  | "cancelled"
  | "failed"
  | "needs_reupload";
export interface CreationSkill {
  name: string;
  description: string;
  compatibility: string;
  allowed_tools: string;
  body: string;
  files: { path: string; content: string }[];
}
export interface CreationReference {
  skill_id: string;
  version_id: string;
  name: string;
  confirmed: boolean;
  available: boolean;
  description?: string;
  compatibility?: string;
  allowed_tools?: string;
  tier?: "curated" | "indexed" | "unknown";
  scan_status?: "scanned" | "unavailable" | "unknown";
  warnings?: number;
}
export type CreationFetch = { url: string; sha256?: string; bytes?: number; status: string };
/**
 * One picture in the conversation. `message_index` is the turn it belongs to:
 * the person's own message when they typed something with it, otherwise the
 * index the model's reply takes.
 *
 * There is no URL here and there will not be one — the platform keeps the
 * digest and refuses the bytes (ADR-066 決策 4). The thumbnail this screen shows
 * is the `File` the browser still holds from the send that created it, so a
 * reload leaves the turn describing a picture it can no longer show.
 */
export interface CreationAttachment {
  message_index: number;
  media_type: string;
  bytes: number;
  sha256: string;
}
/** What the person last confirmed, before a model step overwrote it (05 R-54
 * #4) — present only when that overwrite actually overturned a confirmed
 * value, so the confirm screen has something to compare the new text to. */
export interface CreationModelChange {
  brief?: string;
  acceptance_criteria?: string[];
  sample_input?: string;
}
export interface CreationSnapshot {
  messages: { role: "user" | "assistant" | "tool"; content: string }[];
  brief: string;
  brief_confirmed: boolean;
  acceptance_criteria: string[];
  sample_input?: string;
  model_changed?: CreationModelChange;
  diagram_understanding: string;
  diagram_confirmed: boolean;
  attachments?: CreationAttachment[];
  references: CreationReference[];
  catalog_checked?: boolean;
  duplicates?: CreationReference[];
  pending_materialize?: string;
  duplicate_acknowledged?: boolean;
  adopted?: boolean;
  pending_action: string;
  budget_usd: number;
  reserved_usd: number;
  spent_usd?: number;
  usage_unknown: boolean;
  steps: number;
  tool_calls: number;
  draft_retries?: number;
  run_unmet?: boolean;
  nudges?: number;
  blocked_repeats?: number;
  search_rounds?: number;
  pending_fetch_url?: string;
  fetches?: CreationFetch[];
  draft?: {
    revision: number;
    content_hash: string;
    skill: CreationSkill;
    validation: string;
    blocked: boolean;
  };
  previous_draft?: CreationSnapshot["draft"];
  candidate?: { skill_id: string; version_id: string; run_id?: string; test_case_id?: string };
  diagram_fingerprint?: string;
  diagram_media_type?: string;
  diagram_bytes?: number;
  model?: string;
  prompt_version?: string;
}
export interface CreationSession {
  id: string;
  revision: number;
  state: CreationState;
  snapshot: CreationSnapshot;
  created_at: string;
  updated_at: string;
  expires_at: string;
  deadline: string;
}
export interface CreationLimits {
  min_budget_usd: number;
  max_budget_usd: number;
  max_steps: number;
  max_tool_calls: number;
  call_timeout_seconds: number;
  session_timeout_seconds: number;
  retention_seconds: number;
}
export interface CreationAction {
  command_id: string;
  expected_revision: number;
  kind:
    | "message"
    | "confirm_brief"
    | "confirm_diagram"
    | "select_references"
    | "confirm_references"
    | "materialize"
    | "finalize"
    | "cancel"
    | "diagram"
    | "attach_run"
    | "raise_budget"
    | "confirm_fetch"
    | "decline_fetch"
    | "adopt_reference"
    | "decline_references"
    | "confirm_duplicate";
  message?: string;
  reference_skill_ids?: string[];
  content_hash?: string;
  diagram?: { media_type: string; data: string };
  run_id?: string;
  budget_usd?: number;
}
export const listCreationSessions = () => apiFetch<CreationSession[]>("/creation-sessions");
export const getCreationLimits = () => apiFetch<CreationLimits>("/creation-sessions/limits");
export const getCreationSession = (id: string) =>
  apiFetch<CreationSession>("/creation-sessions/" + id);
export const createCreationSession = (body: { id: string; message: string; budget_usd: number }) =>
  apiFetch<CreationSession>("/creation-sessions", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
export const actOnCreationSession = (id: string, body: CreationAction) =>
  apiFetch<CreationSession>("/creation-sessions/" + id + "/actions", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

/**
 * Whether this deployment shows the interactive creation entry point.
 *
 * Same /me-flag shape as `useGenerateEntryPoint` (ADR-052): a named hook
 * rather than an inline read so `ia.test.ts`'s roster scan can see the mount.
 * Go sends `creation_skill` only when `generate_skill` is also on
 * (apps/platform/internal/entrypoint/api/apiserver/app.go
 * `entryPointFeatures`), and the web still nests it inside `generateExposed`
 * in CreateHub — this hook never widens exposure.
 */
export function useCreationEntryPoint(): boolean {
  const me = useMe();
  return me.data?.features?.creation_skill === true;
}
