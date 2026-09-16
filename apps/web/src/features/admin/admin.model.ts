export const ENTRY_KIND: Record<string, string> = {
  debit: "扣點",
  grant: "授予",
  topup: "儲值",
  adjustment: "更正",
};

export const ACTION_LABEL: Record<string, string> = {
  "skill.access_restrict": "設定受限展示",
  "skill.access_unrestrict": "解除受限展示",
  "skill.redistribution_set": "再散布判定",
  "skill.curation_set": "精選層級",
  "skill.takedown": "下架",
  "credit.grant": "授予點數",
  "dispatch.halted": "停止派送",
  "dispatch.resumed": "恢復派送",
  "account.lookup": "查詢帳號",
  "credit.lookup": "查詢點數",
};

export const COST_KIND: Record<string, string> = {
  creation_step: "創作步驟",
  creation_session: "創作會話",
  search_embedding: "搜尋向量",
  index_enrich: "索引增強",
  review: "評審",
  suggestion: "改善建議",
  generate: "單次生成",
  run: "試跑",
  match_reasons: "搜尋理由",
};
