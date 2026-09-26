---
name: server-log-error-triage
description: Use when a flowchart shows how to handle server log errors, especially when deciding whether repeated errors require escalation. It turns the diagram into step-by-step operational instructions.
---

# 伺服器 log 錯誤處理流程

依照流程圖執行下列步驟：

1. **讀取伺服器 log 檔**
   - 取得目前要檢查的伺服器日誌。
   - 先確認時間範圍、服務名稱或主機名稱，避免讀錯檔案。

2. **篩出 ERROR 行**
   - 從 log 中只保留標記為 `ERROR` 的紀錄。
   - 若需要，整理出錯誤時間、訊息內容、服務名稱與重複次數。

3. **判斷錯誤是否超過 10 筆**
   - 統計篩出的 `ERROR` 行數。
   - 以同一檢查區間或同一事件群組為準，判斷是否 **超過 10 筆**。

4. **如果沒有超過 10 筆**
   - 不建立 Jira 問題單。
   - 直接進入每日摘要紀錄。

5. **如果超過 10 筆**
   - **建立 Jira 問題單**。
   - 在問題單中記錄：
     - 錯誤摘要
     - 發生時間區間
     - 相關 log 片段
     - 影響範圍或服務名稱
     - 錯誤筆數
   - **通知值班工程師**，讓其立即處理。

6. **寫入每日摘要**
   - 不論是否超過 10 筆，最後都要**寫入每日摘要**。
   - 摘要至少包含：
     - 檢查的 log 檔
     - `ERROR` 筆數
     - 是否已建立 Jira
     - 是否已通知值班工程師
     - 重要異常內容或觀察結果

## 執行原則
- 先篩選，再統計，再判斷是否升級處理。
- 超過 10 筆才建立 Jira 並通知值班工程師。
- 每次處理都要留下每日摘要，方便追蹤與交接。
