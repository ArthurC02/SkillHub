import type { IconState } from "../../../shared/ui/StateIcon";
import type {
  CriterionResult,
  DeterministicFinding,
  Evaluation,
  EvidenceMatch,
  EvidenceRef,
  ImprovementSuggestion,
  SuggestionBlockedReason,
} from "../evaluation.service";

export const OVERALL_LABEL: Record<Evaluation["overall"], string> = {
  met: "符合",
  partially_met: "部分符合",
  not_met: "未符合",
  undetermined: "無法判斷",
};

export const CRITERION_LABEL: Record<CriterionResult["result"], string> = {
  passed: "通過",
  failed: "未通過",
  undetermined: "無法判斷",
};

const EVIDENCE_UNVERIFIABLE_PREFIX = "evidence_unverifiable: ";

export function isEvidenceUnverifiable(c: CriterionResult): boolean {
  return c.result === "undetermined" && c.reason.startsWith(EVIDENCE_UNVERIFIABLE_PREFIX);
}

export const SOURCE_LABEL: Record<CriterionResult["source"], string> = {
  rule: "規則判定（平台自己的紀錄）",
  model: "模型評估（不是確定事實）",
  user: "使用者判定",
};

export const FINDING_CATEGORY_LABEL: Record<DeterministicFinding["category"], string> = {
  spec: "規格",
  activation: "啟用",
  execution: "執行",
  effect: "任務效果",
  compatibility: "相容性",
  cost: "成本",
};

export const SEVERITY_LABEL: Record<DeterministicFinding["severity"], string> = {
  error: "錯誤",
  warning: "警告",
  info: "資訊",
};

export const SUGGESTION_CATEGORY_LABEL: Record<ImprovementSuggestion["category"], string> = {
  skill: "Skill 內容問題",
  runtime: "Runtime 問題",
  mcp: "MCP 問題",
  tool: "工具問題",
  dataset: "測試資料問題",
};

export const BLOCKED_REASON_LABEL: Record<SuggestionBlockedReason, string> = {
  path_out_of_bounds: "建議的目標路徑指到套件外面，不能套用。",
  target_changed: "目標檔案已經和建議產生當時不同，這項建議是針對舊內容寫的，不能套用。",
  validation_blocked: "套用後套件會出現阻擋級的規格問題，不能套用。",
  access_restricted: "這個 Skill 目前處於授權受限狀態，平台不重現其套件內容，不能套用。",
  diff_unavailable: "算不出差異。看不到會改什麼就不提供套用。",
};

// Ties keep the first source seen: strict `>` plus Map's insertion order.
export function listSource(results: CriterionResult[]): CriterionResult["source"] | undefined {
  const tally = new Map<CriterionResult["source"], number>();
  for (const c of results) {
    if (isEvidenceUnverifiable(c)) continue;
    tally.set(c.source, (tally.get(c.source) ?? 0) + 1);
  }
  let best: CriterionResult["source"] | undefined;
  for (const [source, n] of tally) if (n > (best ? (tally.get(best) ?? 0) : 1)) best = source;
  return best;
}

export type MatchKey = EvidenceMatch | "unrecorded";

export const MATCH_NOTE: Record<MatchKey, string> = {
  exact: "引文已逐字回驗。",
  normalized:
    "引文已回驗——需要正規化後才比對得上（全形半形、空白、頭尾標點）。原文與引用有細微差異，內容相同。",
  not_found: "這段引文在本次 Run 的可回驗來源裡找不到，因此不作為證據。",
  not_checked:
    "只證明這個檔案存在（路徑、大小、雜湊都在 manifest 上），沒有回驗任何引文——平台不會打開產物內容。",
  unrecorded: "這份報告產生時還沒有記錄引文回驗結果，無法判斷這段引文是否被回驗過。",
};

export const MATCH_WORD: Record<MatchKey, string> = {
  exact: "已逐字回驗",
  normalized: "正規化後比對",
  not_found: "找不到",
  not_checked: "未回驗引文",
  unrecorded: "回驗結果未記錄",
};

export const MATCH_BADGE: Record<MatchKey, string> = {
  exact: "badge",
  normalized: "badge",
  not_found: "badge badge-danger",
  not_checked: "badge badge-unverified",
  unrecorded: "badge badge-unverified",
};

export const MATCH_ICON: Record<MatchKey, IconState> = {
  exact: "pass",
  normalized: "pass",
  not_found: "fail",
  not_checked: "unknown",
  unrecorded: "unknown",
};

export const MATCH_ORDER: MatchKey[] = [
  "exact",
  "normalized",
  "not_found",
  "not_checked",
  "unrecorded",
];

export function matchKey(e: EvidenceRef): MatchKey {
  return e.match ?? "unrecorded";
}

export const KIND_WORD: Record<EvidenceRef["kind"], string> = {
  trace_event: "Trace 事件",
  artifact: "Artifact",
  agent_output: "Agent 輸出",
};
