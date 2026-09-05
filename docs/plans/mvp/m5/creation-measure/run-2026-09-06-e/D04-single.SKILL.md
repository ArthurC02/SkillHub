---
name: server-log-error-triage
description: Use this skill when a flowchart shows how to triage server log errors, especially when deciding whether to escalate after counting ERROR lines. It turns the diagram into a step-by-step operational checklist.
---

# 伺服器 log 錯誤處理流程

依照流程圖執行以下步驟：

1. **讀取伺服器 log 檔**
   - 取得指定時間範圍或指定來源的 server log。
   - 確認你正在處理的是正確的 log 檔或 log 區段。

2. **篩出 `ERROR` 行**
   - 從 log 中只保留包含 `ERROR` 的紀錄。
   - 若需要，先去除重複或明顯無關的訊息，但不要改變錯誤筆數的判定標準。

3. **判斷錯誤是否超過 10 筆**
   - 計算 `ERROR` 行數。
   - 以「超過 10 筆」作為分支條件。

4. **若錯誤超過 10 筆**
   - **建立 Jira 問題單**：整理錯誤摘要、發生時間、影響範圍、相關 log 片段與重現線索。
   - **通知值班工程師**：將問題單資訊與緊急程度傳達給當班負責人。

5. **若錯誤未超過 10 筆**
   - 不建立 Jira 問題單。
   - 不通知值班工程師。
   - 直接進入每日摘要記錄。

6. **寫入每日摘要**
   - 不論是否超過 10 筆，最後都要將處理結果寫入每日摘要。
   - 摘要至少包含：log 來源、`ERROR` 筆數、是否建立 Jira、是否通知值班工程師、以及簡短觀察。

## 執行原則
- 以流程圖為準，先篩選再判斷，再決定是否升級處理。
- 若資訊不足以判定筆數，先補齊 log 範圍與來源，再繼續。
- 若無法實際建立 Jira 或通知工程師，應明確記錄「需人工處理」並保留所有可用資訊。
