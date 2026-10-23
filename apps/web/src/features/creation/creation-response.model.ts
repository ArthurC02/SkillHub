import type {
  CreationMessageRoleEnum,
  CreationReferenceScanStatusEnum,
  CreationReferenceTierEnum,
} from "@skillhub/api-client-ts";
import { contractEnum } from "../../core/api/contract-enum";
import type { CreationSession } from "./creation.service";

type Check = (value: unknown) => boolean;

const REFERENCE_TIERS = contractEnum<CreationReferenceTierEnum>({
  curated: true,
  indexed: true,
  unknown: true,
});
const REFERENCE_SCAN_STATUSES = contractEnum<CreationReferenceScanStatusEnum>({
  scanned: true,
  unavailable: true,
  unknown: true,
});
const MESSAGE_ROLES = contractEnum<CreationMessageRoleEnum>({
  user: true,
  assistant: true,
  tool: true,
});

const text: Check = (value) => typeof value === "string";
const flag: Check = (value) => typeof value === "boolean";
const count: Check = (value) => Number.isSafeInteger(value) && Number(value) >= 0;
const date: Check = (value) => typeof value === "string" && Number.isFinite(Date.parse(value));
const identifier: Check = (value) => typeof value === "string" && value.trim().length > 0;
const optional =
  (check: Check): Check =>
  (value) =>
    value === undefined || check(value);
const array =
  (check: Check): Check =>
  (value) =>
    Array.isArray(value) && value.every(check);
const member =
  (values: readonly string[]): Check =>
  (value) =>
    typeof value === "string" && values.includes(value);

function fields(value: unknown, checks: Record<string, Check>): value is Record<string, unknown> {
  return (
    value !== null &&
    typeof value === "object" &&
    !Array.isArray(value) &&
    Object.entries(checks).every(([key, check]) => check((value as Record<string, unknown>)[key]))
  );
}

const texts = array(text);
const reference: Check = (value) =>
  fields(value, {
    skill_id: identifier,
    version_id: identifier,
    name: text,
    confirmed: flag,
    available: flag,
    description: optional(text),
    compatibility: optional(text),
    allowed_tools: optional(text),
    tier: optional(member(REFERENCE_TIERS)),
    scan_status: optional(member(REFERENCE_SCAN_STATUSES)),
    warnings: optional(count),
  });
const skill: Check = (value) =>
  fields(value, {
    name: text,
    description: text,
    compatibility: text,
    allowed_tools: text,
    body: text,
    files: (files) =>
      files === null || array((file) => fields(file, { path: text, content: text }))(files),
  });
const draft: Check = (value) =>
  fields(value, {
    revision: count,
    content_hash: text,
    skill,
    validation: text,
    blocked: flag,
  });
const snapshot: Check = (value) =>
  fields(value, {
    messages: array((message) =>
      fields(message, {
        role: member(MESSAGE_ROLES),
        content: text,
        created_at: optional(date),
      }),
    ),
    brief: text,
    brief_confirmed: flag,
    acceptance_criteria: (criteria) => criteria === null || texts(criteria),
    sample_input: optional(text),
    challenge_cases: optional(
      array((item) => fields(item, { name: text, prompt: text, criteria: texts })),
    ),
    model_changed: optional((change) =>
      fields(change, {
        brief: optional(text),
        acceptance_criteria: optional(texts),
        sample_input: optional(text),
      }),
    ),
    diagram_understanding: text,
    diagram_confirmed: flag,
    diagram_description: optional(text),
    diagram_description_confirmed: optional(flag),
    diagram_interpretation: optional((diagram) =>
      fields(diagram, {
        nodes: texts,
        conditions: texts,
        branches: texts,
        uncertainties: array((item) =>
          fields(item, { id: identifier, question: text, answer: optional(text) }),
        ),
      }),
    ),
    attachments: optional(
      array((attachment) =>
        fields(attachment, {
          message_index: count,
          media_type: text,
          bytes: count,
          sha256: text,
        }),
      ),
    ),
    references: array(reference),
    duplicates: optional(array(reference)),
    catalog_checked: optional(flag),
    pending_materialize: optional(text),
    duplicate_acknowledged: optional(flag),
    adopted: optional(flag),
    pending_action: text,
    budget_credits: count,
    reserved_credits: count,
    spent_credits: optional(count),
    usage_unknown: flag,
    steps: count,
    tool_calls: count,
    draft_retries: optional(count),
    run_unmet: optional(flag),
    nudges: optional(count),
    blocked_repeats: optional(count),
    search_rounds: optional(count),
    pending_fetch_url: optional(text),
    approved_fetch_url: optional(text),
    fetches: optional(
      array((fetch) =>
        fields(fetch, {
          url: text,
          sha256: optional(text),
          bytes: optional(count),
          status: text,
        }),
      ),
    ),
    draft: optional(draft),
    previous_draft: optional(draft),
    candidate: optional((candidate) =>
      fields(candidate, {
        skill_id: identifier,
        version_id: identifier,
        run_id: optional(identifier),
        test_case_id: optional(identifier),
        challenge_test_case_ids: optional(array(identifier)),
      }),
    ),
    diagram_fingerprint: optional(text),
    diagram_media_type: optional(text),
    diagram_bytes: optional(count),
    model: optional(text),
    prompt_version: optional(text),
  });

export function readCreationSession(value: unknown, id?: string): CreationSession {
  if (
    !fields(value, {
      id: identifier,
      revision: count,
      state: member([
        "queued",
        "working",
        "waiting_input",
        "waiting_confirmation",
        "draft_ready",
        "candidate_ready",
        "saved",
        "cancelled",
        "failed",
        "needs_reupload",
      ]),
      snapshot,
      created_at: date,
      updated_at: date,
      expires_at: date,
      deadline: date,
    }) ||
    (id !== undefined && String(value.id).toLowerCase() !== id.toLowerCase())
  ) {
    throw new Error("創作紀錄的回應不完整，請重新讀取。");
  }
  const session = value as unknown as CreationSession;
  return {
    ...session,
    snapshot: {
      ...session.snapshot,
      acceptance_criteria: session.snapshot.acceptance_criteria ?? [],
    },
  };
}

export function readCreationSessions(value: unknown): CreationSession[] {
  if (!Array.isArray(value)) throw new Error("創作清單的回應不完整，請重新讀取。");
  return value.map((session) => readCreationSession(session));
}
