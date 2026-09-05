# ADR-067：互動式 Skill 創作由 LangGraph 編排，Go 持有會話事實

- 狀態：**Accepted**（規劃已同意，尚未實作）
- 日期：2026-09-05
- 相關：ADR-016、ADR-046、ADR-047、ADR-066、ADR-008、ADR-017

## 背景

既有生成是一次任務描述到版本的路徑，已完成的證據不撤回；它沒有多輪澄清、草稿修訂、跨輪讀圖確認或恢復會話。本 ADR 規劃一條私人互動創作旅程，不能把「多次同 prompt 重試」稱為修訂。

## 決策

Python 以 LangGraph 編排固定 workflow 與有界 ReAct 工具選擇：需求整理與澄清、流程圖／分支理解確認、Catalog 查找與比較、使用者確認參考、確認 brief 與驗收、草稿、Go 驗證、必要時經授權的 Sandbox 試跑、回饋修訂，最後由使用者明確確認保存。資料充分時可短路，不強制每次很多輪。這是規劃選型；LangGraph 尚未安裝，且不採用其原生 durable interrupt／checkpointer。

Go／Postgres 是創作會話唯一事實來源：保存版本化快照（已確認需求、對話或必要摘要與原始輸入引用、參考版本、草稿 revision／雜湊、pending action／confirmation、工具結果、用量、job attempt）與 append-only 事件。每個 Job 由快照重建 Python 圖，回傳工具意圖、待補充、待確認或草稿結果；不保留跨 Job Python checkpoint，也不建立第二套 Run 狀態。Go 以同交易 outbox、expected revision 的 CAS 與 idempotency 管理重啟、取消、逾時及重送；重複或遲到結果不可重做副作用或重建版。

Python 不直接執行不可信 Skill 或 Script、不直連核心 DB、不消費 queue，也不擁有 AuthZ、成本政策或 Go 領域／持久化狀態轉移；圖內節點與下一步仍由 LangGraph 在單一 Job 內決定。所有模型呼叫走 LiteLLM。工具意圖交 Go，由 Go 依 scope 執行檢索、讀取、驗證與試跑，再把結果交回 Python；不把全部 ReAct 決策移回 Go。

最後保存確認綁定使用者所見草稿的 revision 與雜湊，確認後不能再由模型換內容。試跑需先取得使用者對成本、工具、資料與候選版本的明確確認，再建私有不可變候選並走既有 Run；成功不自動完成會話，最後確認選定同一已 materialize 候選，不再次生成或建版。未試跑也可在 Go 套件驗證通過後確認保存，但須明示未經試跑驗證；同一候選重複確認不重建版。草稿可修訂，正式 Version 與 Run 歷史不可覆寫。

流程圖跨輪保存已確認的結構化理解與指紋，不保存原圖位元組；欲重新讀圖必須重新上傳，UI 不得宣稱能回放原圖。參考固定版本且每次使用重驗可讀權限；失效時等待使用者換選，不可默跳。資料不足而無法建出可用 Skill 時，系統可說明限制或替代方案，不得捏造工具能力。對話、草稿與參考均為私有會話資料，受帳號刪除與共享 write fence 管理。公開只可涵蓋使用者明確選定、預覽確認且通過治理的套件內容；私有對話、原圖、原始資料與參考全文不得自動附帶，私有會話保持私有。

互動會話的模型用量與總預算、既有單次生成次數額度及 Run 額度分開。開始對話可能已有模型費，失敗或取消也須如實呈現；會話預算已確認時，由 Go 逐次核准其內的模型呼叫，超限、提權或新試跑才重新要求阻斷確認。實際預算、步數／工具／時間上限、未完成會話保存期限與量測門檻由 [`05` R-45](../plans/05-pending-rulings.md) 具名裁定後才能啟用；未定值不得無上限運行。未知用量不得記為零，重播不撤銷已發生的費用。

## 後果

此決策保留既有單次生成的固定 prompt 重試規則於原路徑；新草稿修訂依已確認回饋與有界編排處理。具體資料表、端點與程式識別字留待契約設計，不以本 ADR 假定已存在的 API。ADR-046 的「不引入 agent framework」只限其舊單次路徑；ADR-066 對舊流程圖處理的規則仍有效，而新會話保存的是已確認解析，不是原圖。ADR-047 的配額與 ADR-066 的輸入／曝光邊界均不因本 ADR 自動解除。

## 承接與驗證

本次只更新產品規劃，不解除 `01` §10 的功能凍結與 M5 曝光限制。允收由 [`02` GEN-007～012](../plans/02-specifications-and-acceptance-criteria.md) 持有；實作由 [`03` GEN-016～023](../plans/03-work-items.md) 承接，全部尚未完成；[`04` 丙-166](../plans/04-backlog-and-handoffs.md) 追蹤交付。對話、讀圖與參考三種入口均須以多輪修正、恢復、授權試跑與精確保存證據驗收，mock 測試不能取代真人與實際模型證據。

**2026-09-06 補記（不改寫上文）**：決策段第一段的「LangGraph 尚未安裝」自 2026-09-05 `d8132c4` 起不成立——`langgraph` 已是 `apps/llm` 的釘選依賴，圖在 `creation.py` 內每個 Job 重建、仍不用原生 checkpointer，與本 ADR 的邊界一致；狀態句「規劃已同意，尚未實作」以 `01` §10 與 `04` 丙-166 為準。第六段「超限、提權或新試跑才重新要求阻斷確認」以 `raise_budget` 命令落地（`05` R-46）；R-45 的數值於同日由負責人授權代理定值。

框架概念參照官方 [workflows and agents](https://docs.langchain.com/oss/python/langgraph/workflows-agents) 與 [interrupts](https://docs.langchain.com/oss/python/langgraph/interrupts)；本案刻意不採用後者的跨程序持久化模式，以維持 Go 唯一持久化邊界。
