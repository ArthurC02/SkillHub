---
name: server-log-error-triage
description: 'Use when you need to process a server log, filter ERROR entries, and decide whether the incident warrants escalation. It follows a simple triage flow: count errors, create a Jira issue and notify the on-call engineer if the threshold is exceeded, then record a daily summary.'
---

# 伺服器 log 錯誤處理流程

依照下列流程處理伺服器 log：

1. **讀取伺服器 log 檔**
   - 取得當日或指定期間的伺服器 log。
   - 確認 log 來源、時間範圍與檔案完整性。

2. **篩出 `ERROR` 行**
   - 從 log 中只保留包含 `ERROR` 的紀錄。
   - 統計錯誤筆數，並保留可用於後續追查的關鍵資訊，例如時間、服務名稱、錯誤訊息與重複次數。

3. **判斷錯誤是否超過 10 筆**
   - 若 `ERROR` 筆數 **不超過 10 筆**：
     - 不建立 Jira 問題單。
     - 不通知值班工程師。
     - 直接進入每日摘要。
   - 若 `ERROR` 筆數 **超過 10 筆**：
     - 繼續執行第 4、5 步。

4. **建立 Jira 問題單**
   - 建立一張新的 Jira issue。
   - 在內容中整理：錯誤總數、主要錯誤樣式、發生時間區間、受影響服務與必要的 log 摘要。

5. **通知值班工程師**
   - 將 Jira 單號與重點錯誤摘要通知值班工程師。
   - 通知內容應包含：是否持續發生、是否影響服務、以及需要優先查看的 log 位置。

6. **寫入每日摘要**
   - 不論是否超過 10 筆，最後都要寫入每日摘要。
   - 摘要至少包含：
     - log 檢查時間
     - `ERROR` 總數
     - 是否超過 10 筆
     - 是否建立 Jira
     - 是否通知值班工程師
     - 重要觀察或後續建議

## 執行原則
- 先完成 log 篩選與統計，再決定是否升級處理。
- 若資訊不足以判斷是否超過 10 筆，先補齊統計後再繼續。
- 每次處理都要留下可追蹤的摘要紀錄。
