import { IN_FLIGHT_RUN_STATUSES, type RunStatus } from "./trace.service";

export type RunActivityGroup = "needs_attention" | "in_flight" | "recent";

const RUN_STATUSES_NEEDING_ATTENTION = new Set(["failed", "timed_out"]);
const RUN_VERDICTS_NEEDING_ATTENTION = new Set([
  "not_met",
  "partially_met",
  "undetermined",
  "evaluation_failed",
]);

export const RUN_STATUS_LABEL: Record<RunStatus, string> = {
  queued: "排隊中",
  provisioning: "環境準備中",
  preparing: "準備中",
  running: "執行中",
  evaluating: "評估中",
  succeeded: "執行完成",
  failed: "執行失敗",
  cancelled: "已取消",
  timed_out: "執行逾時",
};

export function runStatusLabel(status: string): string {
  return RUN_STATUS_LABEL[status as RunStatus] ?? status;
}

export function runActivityGroup(status: string, verdict: string): RunActivityGroup {
  if (IN_FLIGHT_RUN_STATUSES.has(status)) return "in_flight";
  if (RUN_STATUSES_NEEDING_ATTENTION.has(status)) return "needs_attention";
  if (RUN_VERDICTS_NEEDING_ATTENTION.has(verdict)) return "needs_attention";
  return "recent";
}

export function runAttentionAction(status: string, verdict: string): string {
  if (RUN_STATUSES_NEEDING_ATTENTION.has(status)) return "查看原因";
  if (verdict === "evaluation_failed") return "查看評估狀態";
  return "檢視證據";
}

export const CLEANUP_BADGE: Record<string, string> = {
  pending: "badge badge-unverified",
  cleaning_up: "badge badge-unverified",
  cleaned: "badge",
  failed: "badge badge-danger",
};
