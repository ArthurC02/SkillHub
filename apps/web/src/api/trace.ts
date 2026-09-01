import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";

/**
 * GET /runs/{id}/trace (contracts/openapi/public.yaml, TRACE-006/007).
 *
 * One endpoint, two modes: the general summary and the masked raw events. Both
 * are session-scoped — a trace is user data and the workspace comes from the
 * session, never from anything this file sends (iron rule 3).
 *
 * There is no unmasked mode to ask for. Masking happens before storage, so the
 * plaintext this would render does not exist anywhere to be fetched (TRACE-005).
 */

export type TraceMode = "general" | "advanced";

export type TraceSummary = {
  run_id: string;
  /** From the runs table, which is the only authority on run state (iron rule 5). */
  status: string;
  status_reason?: string;
  /** False when a producer's sequence has a hole: some events were lost. */
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
    /** null means the gateway did not report a cost. Never render it as 0. */
    cost_usd: number | null;
    cost_source?: string;
  };
  /**
   * The run's progress, from the authoritative transition history.
   *
   * Two fields rather than one pre-joined `"<status>: <reason>"` string since
   * 2026-09-01: the server was writing a status for a reader, which is this
   * app's job and always was — `runStatusLabel` sits four lines above the
   * progress list on the same screen, so it showed 「執行完成」 and `succeeded:`
   * at once (04 丙-115 ①).
   *
   * `reason` is not always the platform's own words: some are relayed verbatim
   * from the provider that ran the workload, and some are a Go error's own text.
   * Neither can be translated here, and neither is invented — an English
   * sentence in this field is the mark of a relayed one.
   */
  steps: { status: string; reason?: string }[];
  /**
   * When this run last produced anything. Absent means nothing has arrived yet —
   * a real state for a run still being provisioned, not a missing field, and it
   * is worded rather than rendered as a blank or as "0 秒前" (設計 §2.12).
   */
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
  /** Arrived after the run had already reached a terminal state (TRACE-008). */
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

/**
 * The contract's `RunStatus`, once.
 *
 * There used to be FOUR hand-written subsets of it and no assertion tying any
 * of them together: `RUN_STATUS_LABEL` (9 values), `CANCELLABLE` (5),
 * `TERMINAL_RUN_STATUSES` (4) and `REASON_EXPECTED` (3). Today their union is
 * right by coincidence, and the consequence of a tenth status is not one bug
 * but a chain: the label table prints English into a Chinese sentence, this set
 * says the run never ended, `CANCELLABLE` says it cannot be cancelled — so
 * `InFlight` never disappears and `useTrace` polls every three seconds forever,
 * with all three tables green.
 *
 * So: one list, and the terminal set is **the complement of the in-flight one**
 * rather than a second hand-written list. A status added to the contract and to
 * `RUN_STATUSES` but not to `IN_FLIGHT_RUN_STATUSES` becomes terminal, which is
 * the safe direction — polling stops. `contract.test.ts` compares this list
 * against the generated `RunStatusEnum` so the list itself cannot drift.
 */
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

/**
 * Still going. These are exactly the states a Run can be cancelled from
 * (`RunTrace.tsx` used to keep its own copy of this under the name
 * `CANCELLABLE`) and exactly the ones that are not terminal.
 */
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
    // Same as every hook in api/: a failed read is shown as a failed read. With
    // the default three retries `isLoading` stays false through the backoff
    // while `isError` is not yet true, so a logged-out visitor watched
    // 「載入執行紀錄中…」 for ~7 seconds before the 401 surfaced (資訊架構 IA-6).
    // Orthogonal to `refetchInterval` below: polling resumes on its own, a
    // retry only stretches the first failure.
    retry: false,
    // A run in flight keeps producing events, and NFR-004 wants them on screen
    // within seconds of being produced. Polling rather than a stream: there is
    // no push channel, and one cheap request every few seconds is the whole
    // requirement.
    refetchInterval: (query) => {
      if (active === false) return false;
      if (mode === "general") {
        const data = query.state.data as TraceSummary | undefined;
        if (data?.status && TERMINAL_RUN_STATUSES.has(data.status)) return false;
      }
      return 3000;
    },
    // An advanced page may contain 1,000 payloads. Drop pages as soon as the
    // user moves away; Back simply refetches that cursor.
    gcTime: mode === "advanced" ? 0 : undefined,
  });
}
