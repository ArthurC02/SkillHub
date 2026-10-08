import type { PlatformAgentProposal, ProposalStatus } from "../../admin.service";

export const PROPOSAL_STATUS: Record<ProposalStatus, string> = {
  proposed: "待核准",
  approved: "已核准，等待執行",
  rejected: "已駁回",
  expired: "已逾期作廢",
  running: "執行中",
  succeeded: "已執行完成",
  failed: "執行失敗",
};

export const PROPOSAL_TIER: Record<PlatformAgentProposal["tier"], string> = {
  read_only: "唯讀",
  reversible: "可逆",
  destructive: "破壞性",
};

const JOB_NAME: Record<string, string> = {
  "purge-accounts": "清除到期刪除的帳號",
  "purge-run-artifacts": "清除過了保存期的試跑產物",
  "purge-datasets": "清除過了保存期的資料集",
  "purge-deleted-skills": "清除已刪除的 Skill",
  "collect-objects": "回收沒人用的套件檔",
  "check-sources": "檢查匯入來源",
  "purge-audit": "清除過了保存期的稽核紀錄",
  "purge-feedback": "清除過了保存期的回饋",
  "rotate-partitions": "輪替分割表",
};

export function actionLabel(action: string): string {
  const job = action.replace(/^run-/, "");
  return `立刻補跑「${JOB_NAME[job] ?? job}」`;
}

const PREVIEW_KEY: Record<string, string> = {
  accounts_past_grace: "過了寬限期的帳號",
  expired_sessions: "過期的登入",
  run_outputs_past_retention: "過了保存期的試跑產物",
  run_upload_intents_due: "該清掉的產物上傳殘留",
  datasets_past_retention: "過了保存期的資料集",
  dataset_cleanups_due: "該清掉的資料集殘留",
  deleted_skills_past_grace: "過了寬限期的已刪除 Skill",
  orphan_objects: "沒人用的套件檔",
  sources_to_check: "要檢查的匯入來源",
  audit_events_past_retention: "過了保存期的稽核紀錄",
  feedback_reports_past_retention: "過了保存期的回饋",
  trace_partitions_created: "要新建的 Trace 分割表",
  trace_partitions_dropped: "要刪除的 Trace 分割表",
  trace_events_removed: "會刪除的 Trace 事件",
  analytics_partitions_created: "要新建的分析分割表",
  analytics_partitions_dropped: "要刪除的分析分割表",
  analytics_events_removed: "會刪除的分析事件",
};

export function previewLine(item: { key: string; count: number; at_most: boolean }): string {
  return `${PREVIEW_KEY[item.key] ?? item.key}：${item.at_most ? "最多 " : ""}${item.count} 筆`;
}
