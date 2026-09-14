import type { RunStatus } from "../api/trace";

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

export const CLEANUP_BADGE: Record<string, string> = {
  pending: "badge badge-unverified",
  cleaning_up: "badge badge-unverified",
  cleaned: "badge",
  failed: "badge badge-danger",
};
