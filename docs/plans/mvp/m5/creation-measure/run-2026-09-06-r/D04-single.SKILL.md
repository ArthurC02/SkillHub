---
name: server-log-error-triage
description: 'Use when you need to process a server log, filter ERROR entries, and decide whether the incident warrants escalation. It follows a simple triage flow: count errors, create a Jira issue and notify the on-call engineer if the threshold is exceeded, then record a daily summary.'
---

# 伺服器 log 錯誤處理流程

依照下列流程處理伺服器 log：

1. **讀取伺服器 log 檔**
   - 取得要檢查的 log 檔內容。
   - 若 log 檔無法取得，先記錄無法讀取的原因，並停止後續判斷。

2. **篩出 ERROR 行**
   - 從 log 中只保留包含 `ERROR` 的行。
   - 統計 ERROR 行數，作為後續判斷依據。

3. **判斷錯誤是否超過 10 筆**
   - 若 ERROR 行數 **大於 10**，視為需要升級處理。
   - 若 ERROR 行數 **不超過 10**，直接進入每日摘要。

4. **若超過 10 筆：建立 Jira 問題單**
   - 建立一張 Jira issue，內容至少包含：
     - log 檔名稱或來源
     - ERROR 行數
     - 主要錯誤摘要或代表性錯誤訊息
     - 發生時間範圍（若可取得）
   - 讓問題單能被後續追蹤與處理。

5. **通知值班工程師**
   - 將 Jira 問題單資訊通知值班工程師。
   - 通知內容應包含：
     - 問題單連結或編號
     - ERROR 數量
     - 是否已確認超過門檻

6. **寫入每日摘要**
   - 不論是否超過 10 筆，最後都要寫入每日摘要。
   - 摘要至少包含：
     - 檢查的 log 檔
     - ERROR 行數
     - 是否建立 Jira
     - 是否通知值班工程師
     - 任何值得注意的錯誤重點

## 執行原則
- 只根據 log 內容與 ERROR 數量做判斷。
- 若資訊不足，先記錄缺少哪些資料，再進行可完成的部分。
- 每次處理都要留下可追蹤的摘要紀錄。
