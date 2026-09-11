import { API_BASE_URL, apiFetch } from "./client";
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
export interface CreationAttachment {
  message_index: number;
  media_type: string;
  bytes: number;
  sha256: string;
}
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
  budget_credits: number;
  reserved_credits: number;
  spent_credits?: number;
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
  min_budget_credits: number;
  max_budget_credits: number;
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
    | "confirm_duplicate"
    | "stop_step";
  message?: string;
  reference_skill_ids?: string[];
  content_hash?: string;
  diagram?: { media_type: string; data: string };
  run_id?: string;
  budget_credits?: number;
}
export const listCreationSessions = () => apiFetch<CreationSession[]>("/creation-sessions");
export const getCreationLimits = () => apiFetch<CreationLimits>("/creation-sessions/limits");
export const getCreationSession = (id: string) =>
  apiFetch<CreationSession>("/creation-sessions/" + id);
export const createCreationSession = (body: {
  id: string;
  message: string;
  budget_credits: number;
}) =>
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

export function useCreationEntryPoint(): boolean {
  const me = useMe();
  return me.data?.features?.creation_skill === true;
}

export function streamCreationSession(
  id: string,
  onSession: (s: CreationSession) => void,
  onOpen: (live: boolean) => void,
): () => void {
  if (typeof EventSource === "undefined") {
    onOpen(false);
    return () => {};
  }
  const source = new EventSource(API_BASE_URL + "/creation-sessions/" + id + "/events", {
    withCredentials: true,
  });
  source.onopen = () => onOpen(true);
  source.onmessage = (e) => {
    try {
      onSession(JSON.parse(e.data) as CreationSession);
    } catch {
      // Ignore a half-written frame; the poll underneath will catch up.
    }
  };
  source.onerror = () => {
    // The browser retries the connection on its own; this just hands control
    // back to the poll until an open event says the stream is live again.
    onOpen(false);
  };
  return () => {
    onOpen(false);
    source.close();
  };
}
