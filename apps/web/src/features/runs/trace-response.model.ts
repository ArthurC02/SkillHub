import type { TraceAdvanced, TraceSummary } from "./trace.service";

export const RUN_STATUSES = [
  "queued",
  "provisioning",
  "preparing",
  "running",
  "evaluating",
  "succeeded",
  "failed",
  "cancelled",
  "timed_out",
] as const;

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function isText(value: unknown): value is string {
  return typeof value === "string";
}

function isOptionalText(value: unknown): boolean {
  return value === undefined || isText(value);
}

function isList(value: unknown, item: (value: unknown) => boolean): boolean {
  return Array.isArray(value) && value.every(item);
}

function isTrace(value: unknown, runId: string): value is Record<string, unknown> {
  return (
    isRecord(value) &&
    isText(value.run_id) &&
    value.run_id.toLowerCase() === runId.toLowerCase() &&
    typeof value.complete === "boolean"
  );
}

function isSkill(value: unknown): boolean {
  return (
    isRecord(value) &&
    isText(value.name) &&
    (value.decision === "activated" || value.decision === "skipped") &&
    isOptionalText(value.reason)
  );
}

function isError(value: unknown): boolean {
  return isRecord(value) && isText(value.category) && isText(value.code) && isText(value.message);
}

function isStep(value: unknown): boolean {
  return isRecord(value) && isText(value.status) && isOptionalText(value.reason);
}

function isQuestion(value: unknown): boolean {
  return (
    isRecord(value) &&
    isText(value.question) &&
    isText(value.why) &&
    (value.options === undefined || isList(value.options, isText))
  );
}

function isToolCalls(value: unknown): boolean {
  return (
    isRecord(value) &&
    ["total", "succeeded", "failed", "total_duration_ms", "slowest_duration_ms"].every((key) =>
      Number.isSafeInteger(value[key]),
    ) &&
    isOptionalText(value.slowest_tool)
  );
}

function isUsage(value: unknown): boolean {
  return (
    isRecord(value) &&
    isOptionalText(value.model) &&
    Number.isSafeInteger(value.input_tokens) &&
    Number.isSafeInteger(value.output_tokens) &&
    (value.cost_credits === null || Number.isSafeInteger(value.cost_credits)) &&
    (value.cost_source === undefined ||
      value.cost_source === "gateway" ||
      value.cost_source === "estimated")
  );
}

export function isTraceSummary(value: unknown, runId: string): value is TraceSummary {
  return (
    isTrace(value, runId) &&
    isText(value.status) &&
    (RUN_STATUSES as readonly string[]).includes(value.status) &&
    isOptionalText(value.status_reason) &&
    isList(value.skills, isSkill) &&
    Number.isSafeInteger(value.skills_total) &&
    Number.isSafeInteger(value.resources_read) &&
    isToolCalls(value.tool_calls) &&
    isList(value.errors, isError) &&
    Number.isSafeInteger(value.errors_total) &&
    typeof value.summary_truncated === "boolean" &&
    isOptionalText(value.final_output) &&
    (value.questions === undefined || isList(value.questions, isQuestion)) &&
    (value.usage === undefined || isUsage(value.usage)) &&
    isList(value.steps, isStep) &&
    (value.last_event_at === undefined ||
      (isText(value.last_event_at) && Number.isFinite(Date.parse(value.last_event_at))))
  );
}

function isStream(value: unknown): boolean {
  return (
    isRecord(value) &&
    isText(value.emitted_by) &&
    ["attempt", "received", "highest_seq", "missing_count", "late_events"].every((key) =>
      Number.isSafeInteger(value[key]),
    ) &&
    (value.missing_seq === undefined || isList(value.missing_seq, Number.isSafeInteger))
  );
}

function isEvent(value: unknown): boolean {
  return (
    isRecord(value) &&
    isText(value.event_id) &&
    Number.isSafeInteger(value.attempt) &&
    Number.isSafeInteger(value.seq) &&
    isText(value.occurred_at) &&
    Number.isFinite(Date.parse(value.occurred_at)) &&
    isText(value.emitted_by) &&
    isText(value.type) &&
    isOptionalText(value.status) &&
    (value.late === undefined || typeof value.late === "boolean") &&
    isList(value.masked_fields, isText) &&
    "payload" in value
  );
}

export function isTraceAdvanced(value: unknown, runId: string): value is TraceAdvanced {
  return (
    isTrace(value, runId) &&
    isList(value.streams, isStream) &&
    isList(value.events, isEvent) &&
    Number.isSafeInteger(value.next_after) &&
    typeof value.has_more === "boolean"
  );
}
