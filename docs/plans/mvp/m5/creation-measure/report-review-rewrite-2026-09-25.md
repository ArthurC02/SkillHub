# 試跑回饋修訂：評估被遮蔽與正文丟失

## 結論與範圍

[完整會話驗證](report-live-enrichment-2026-09-25.md) 觀察到「模型說已改正文，但內容沒有變」。本次確認並修正兩個決定性的接線問題，不把問題全歸因於模型能力，也不升級模型。

1. review 原本只看最後一筆 tool 訊息。Go 的靜態驗證或追問提示排在評估後面，就不啟動診斷／重寫。現在找最新一筆評估；最新評估已通過便停止，不回頭採用舊失敗。
2. 獨立重寫結果原本只套用於 `draft` outcome。模型選擇先送 `validate_draft` 時，仍帶舊正文。現在兩條攜帶草稿的路徑都使用已重寫的正文。

沒有新增模型呼叫類型、依賴或權限；原有單步逾時與用量彙總維持。修復的是應該執行卻被跳過、或已產生卻被丟失的結果。Go 仍負責狀態、驗證與工具執行。

## 決定性測試與突變

`test_review_after_an_unmet_trial_names_the_edits_before_rewriting` 以決策表涵蓋「評估後有／無靜態驗證」×「直接回稿／先驗證草稿」四格，斷言診斷、重寫、決策共三次呼叫、返回正文為重寫結果，以及三次用量皆被加總。

- 恢復只看最後 tool 訊息：兩個有後續驗證的案例 FAIL。
- 禁止重寫結果進入 `validate_draft`：兩個先驗證案例 FAIL，返回舊正文。
- `test_review_uses_the_latest_trial_even_after_static_validation`：先失敗、後成功、再靜態驗證，只應呼叫一次決策模型。改為繼續搜尋更早的失敗 → FAIL，實際呼叫兩次。

突變全部還原。完整 LLM suite：306 passed、6 skipped，exit 0；四個 live-gateway 測試未啟用，兩個 liveness inline 回應沒有 model 可供 enum 對帳。Ruff lint 通過、26 個檔案格式通過。這不是完整部署 E2E 的替代品。

## 真實模型探針

以先前 CSV 失敗形狀重建一份快照：正文欠缺三個列數統計，tool 評估判該條件 failed，接著有使用者修訂要求與 Go 靜態驗證訊息。這是**重建 fixture 加真實模型呼叫**，不是新的真實 Run 或真人答問，也不是重播原會話的完整快照。

直接執行產品 LangGraph（含 prompt preparation、review、tool routing、response rendering），經 LiteLLM 使用 `gpt-5.4-mini`；Virtual Key 預算 US$0.25、24 小時，步驟逾時 90 秒。沒有啟動 Sandbox 或寫入產品資料庫。

結果：outcome=`tool_intent`、正文確實改變、`LIVE_REVISION_PASS`、exit 0；8.39 秒，5798 input tokens、644 output tokens、服務用量回應列報 US$0.0072465。實際新增的正文：

> 結果必須依序列出「資料總列數」、「有效數字列數」、「缺值列數」三個欄位，且每次摘要都要出現這三項。

本次恰好走到原本會丟失重寫內容的先驗證分支。分母只有 1 份合成快照，能佐證修復後路徑運作，**不能宣稱改稿成功率、跨任務穩定性或一般模型幻覺已解決**。原會話歷史報告不回改；真人答問與正式隔離部署證據仍由 `04` 追蹤。
