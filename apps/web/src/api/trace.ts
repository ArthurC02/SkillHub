import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";

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

export type TraceEvent = {
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
  events: TraceEvent[];
  next_after: number;
  has_more: boolean;
};

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

export function useTrace<M extends TraceMode>(runId: string, mode: M, active?: boolean, after = 0) {
  const queryKey = ["trace", runId, mode, mode === "advanced" ? after : 0] as const;
  return useQuery({
    queryKey,
    queryFn: async () => {
      if (mode === "general") {
        return apiFetch<TraceSummary>(`/runs/${runId}/trace`) as Promise<
          M extends "advanced" ? TraceAdvanced : TraceSummary
        >;
      }
      return apiFetch<TraceAdvanced>(
        `/runs/${runId}/trace?mode=advanced&after=${after}`,
      ) as Promise<M extends "advanced" ? TraceAdvanced : TraceSummary>;
    },
    retry: false,
    refetchInterval: (query) => {
      if (active === false) return false;
      if (mode === "general") {
        const data = query.state.data as TraceSummary | undefined;
        if (data?.status && TERMINAL_RUN_STATUSES.has(data.status)) return false;
      }
      return 3000;
    },
    gcTime: mode === "advanced" ? 0 : undefined,
  });
}
