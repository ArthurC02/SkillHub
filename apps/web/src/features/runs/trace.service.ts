import { useQuery } from "@tanstack/react-query";
import { apiFetch, isLastingReadFailure } from "../../core/api/client";
import { queryKeys } from "../../core/api/queryKeys";
import { isTraceAdvanced, isTraceSummary, RUN_STATUSES } from "./trace-response.model";
export { RUN_STATUSES } from "./trace-response.model";

export type TraceMode = "general" | "advanced";

export type TraceSummary = {
  run_id: string;
  status: string;
  status_reason?: string;
  complete: boolean;
  skills: { name: string; decision: string; reason?: string }[];
  skills_total: number;
  resources_read: number;
  tool_calls: {
    total: number;
    succeeded: number;
    failed: number;
    total_duration_ms: number;
    slowest_duration_ms: number;
    slowest_tool?: string;
  };
  errors: { category: string; code: string; message: string }[];
  errors_total: number;
  summary_truncated: boolean;
  final_output?: string;
  questions?: { question: string; why: string; options?: string[] }[];
  usage?: {
    model?: string;
    input_tokens: number;
    output_tokens: number;
    cost_credits: number | null;
    cost_source?: string;
  };
  steps: { status: string; reason?: string }[];
  last_event_at?: string;
};

export type TraceStream = {
  attempt: number;
  emitted_by: string;
  received: number;
  highest_seq: number;
  missing_count: number;
  missing_seq?: number[];
  late_events: number;
};

export type TraceEventView = {
  event_id: string;
  attempt: number;
  seq: number;
  occurred_at: string;
  emitted_by: string;
  type: string;
  status?: string;
  late?: boolean;
  masked_fields: string[];
  payload: unknown;
};

export type TraceAdvanced = {
  run_id: string;
  complete: boolean;
  streams: TraceStream[];
  events: TraceEventView[];
  next_after: number;
  has_more: boolean;
};

export type RunStatus = (typeof RUN_STATUSES)[number];

export const IN_FLIGHT_RUN_STATUSES = new Set<string>([
  "queued",
  "provisioning",
  "preparing",
  "running",
  "evaluating",
]);

export const TERMINAL_RUN_STATUSES = new Set<string>(
  RUN_STATUSES.filter((status) => !IN_FLIGHT_RUN_STATUSES.has(status)),
);

class InvalidTraceResponseError extends Error {}

export function useTrace<M extends TraceMode>(runId: string, mode: M, active?: boolean, after = 0) {
  return useQuery({
    queryKey: queryKeys.trace.page(runId, mode, mode === "advanced" ? after : 0),
    queryFn: async () => {
      const query = mode === "advanced" ? `?mode=advanced&after=${after}` : "";
      const result = await apiFetch<unknown>(`/runs/${runId}/trace${query}`);
      if (!(mode === "general" ? isTraceSummary(result, runId) : isTraceAdvanced(result, runId)))
        throw new InvalidTraceResponseError("Invalid trace response");
      return result as M extends "advanced" ? TraceAdvanced : TraceSummary;
    },
    refetchInterval: (query) => {
      if (query.state.error instanceof InvalidTraceResponseError) return false;
      if (active === false || isLastingReadFailure(query.state.error)) return false;
      if (mode === "general") {
        const data = query.state.data as TraceSummary | undefined;
        if (data?.status && TERMINAL_RUN_STATUSES.has(data.status)) return false;
      }
      return 3000;
    },
    gcTime: mode === "advanced" ? 0 : undefined,
  });
}
