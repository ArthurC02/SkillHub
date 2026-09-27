import type { FeedbackKind } from "./feedback.service";

export const KIND_LABEL: Record<FeedbackKind, string> = {
  blocking_issue: "有東西擋住我，做不下去",
  need_signal: "我想要的東西，這裡沒有",
};

export const KIND_NOTE: Record<FeedbackKind, string> = {
  blocking_issue: "例如：按了沒有反應、看不懂錯誤訊息、卡在某一步過不去。",
  need_signal: "例如：想用的功能不存在、額度不夠、還沒被邀請就想試。",
};
