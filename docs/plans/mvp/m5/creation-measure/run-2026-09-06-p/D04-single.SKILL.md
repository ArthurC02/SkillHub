---
name: server-log-error-triage
description: Use this skill when you need to triage server log files for ERROR entries and decide whether the issue escalation threshold has been exceeded. It follows the flow of filtering errors, checking whether there are more than 10 errors, creating a Jira issue and notifying the on-call engineer if needed, then writing the daily summary.
---

# 伺服器 log 錯誤處理流程

依照下列流程處理伺服器 log：

1. **讀取伺服器 log 檔**
   - 取得要檢查的 log 檔內容。
   - 若 log 檔無法取得，先記錄無法讀取的原因，並停止後續判斷。

2. **篩出 `ERROR` 行**
   - 從 log 中只保留包含 `ERROR` 的行。
   - 計算 `ERROR` 行數量。

3. **判斷錯誤是否超過 10 筆**
   - 若 `ERROR` 行數量 **大於 10**，走「是」分支。
   - 若 `ERROR` 行數量 **不大於 10**，走「否」分支。

4. **若超過 10 筆：建立 Jira 問題單**
   - 建立一張 Jira issue，內容至少包含：
     - log 檔名稱或來源
     - `ERROR` 行數量
     - 代表性錯誤摘要或前幾筆錯誤內容
     - 發生時間範圍（若可從 log 推得）
   - 確保問題單能讓後續值班工程師快速理解問題。

5. **通知值班工程師**
   - 將 Jira 問題單資訊通知值班工程師。
   - 通知內容應包含問題單連結或編號，以及錯誤摘要。

6. **寫入每日摘要**
   - 不論是否超過 10 筆，都要寫入每日摘要。
   - 每日摘要至少記錄：
     - log 檔來源
     - `ERROR` 行數量
     - 是否超過 10 筆
     - 若有建立 Jira，記錄 Jira 編號
     - 若有通知值班工程師，記錄通知結果

## 執行原則
- 只根據 log 中實際出現的內容判斷，不要臆測錯誤數量。
- 若 `ERROR` 行數量不超過 10 筆，直接進入每日摘要，不建立 Jira，也不通知值班工程師。
- 若 `ERROR` 行數量超過 10 筆，先建立 Jira，再通知值班工程師，最後寫入每日摘要。
