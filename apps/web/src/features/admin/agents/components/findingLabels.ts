import type { FindingStatus, PlatformAgentFindingEvent } from "../../admin.service";

export const FINDING_STATUS: Record<FindingStatus, string> = {
  open: "待處理",
  acknowledged: "處理中",
  resolved: "已解決",
  dismissed: "已忽略",
  recovered: "已自行恢復",
};

export const FINDING_EVENT: Record<PlatformAgentFindingEvent["kind"], string> = {
  opened: "日報首次回報",
  seen: "日報再次回報",
  reopened: "重新打開",
  recovered: "日報不再提到，自行恢復",
  acknowledged: "開始處理",
  resolved: "標記已解決",
  dismissed: "忽略",
};

export const FINDING_MOVES: Record<FindingStatus, { to: FindingStatus; label: string }[]> = {
  open: [
    { to: "acknowledged", label: "我來處理" },
    { to: "resolved", label: "標記已解決" },
    { to: "dismissed", label: "忽略這件事" },
  ],
  acknowledged: [
    { to: "resolved", label: "標記已解決" },
    { to: "dismissed", label: "忽略這件事" },
  ],
  resolved: [{ to: "open", label: "重新打開" }],
  dismissed: [{ to: "open", label: "重新打開" }],
  recovered: [
    { to: "resolved", label: "標記已解決" },
    { to: "open", label: "重新打開" },
  ],
};
