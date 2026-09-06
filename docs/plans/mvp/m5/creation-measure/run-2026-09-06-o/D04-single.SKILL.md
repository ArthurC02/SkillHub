---
name: server-log-error-triage
description: 'Use when a flowchart shows how to handle server log errors, especially when deciding whether error volume warrants escalation. It guides the agent to follow the chart: read logs, filter ERROR lines, check the count threshold, create a Jira issue and notify the on-call engineer if needed, then write the daily summary.'
---

# 伺服器 log 錯誤處理流程

依照流程圖執行，不要自行跳步。

## 步驟

1. **讀取伺服器 log 檔**
   - 取得當前要檢查的伺服器日誌內容。
   - 若沒有 log 檔可讀，先回報缺少資料，無法繼續。

2. **篩出 `ERROR` 行**
   - 從 log 中只保留包含 `ERROR` 的行。
   - 以這些行作為後續判斷依據。

3. **判斷錯誤是否超過 10 筆**
   - 計算 `ERROR` 行數量。
   - 若**超過 10 筆**，走「是」分支。
   - 若**不超過 10 筆**，走「否」分支。

4. **若超過 10 筆：建立 Jira 問題單**
   - 建立一張 Jira issue，內容應包含：
     - 錯誤摘要
     - `ERROR` 行數量
     - 相關時間範圍或關鍵 log 片段
     - 影響說明（若可從 log 推得）
   - 若缺少 Jira 權限或必要資訊，明確指出無法完成建立。

5. **若超過 10 筆：通知值班工程師**
   - 將問題單資訊與錯誤摘要通知值班工程師。
   - 若無法直接通知，記錄應通知的對象與原因。

6. **寫入每日摘要**
   - 不論是否超過 10 筆，最後都要寫入每日摘要。
   - 摘要至少包含：
     - 檢查的 log 範圍或來源
     - `ERROR` 行數量
     - 是否超過 10 筆
     - 若有，Jira 單號與通知結果

## 輸出原則

- 嚴格依照流程圖：先讀 log，再篩 `ERROR`，再判斷數量，最後進入對應處理並寫每日摘要。
- 不要自行新增未出現在流程圖中的處置步驟。
- 若資訊不足以完成某一步，說明缺少什麼資料，而不是臆測。
