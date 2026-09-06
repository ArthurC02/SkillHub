---
name: server-log-error-triage
description: Use when a flowchart shows how to handle server log errors, especially to decide whether to escalate after filtering ERROR lines and counting incidents. It guides the agent to follow the chart step by step and produce the appropriate triage actions.
---

# 伺服器 log 錯誤處理流程

依照流程圖執行，不要自行新增步驟。

## 流程

1. **讀取伺服器 log 檔**
   - 取得指定時間範圍或指定來源的伺服器 log。
   - 若 log 檔無法取得，先回報缺少資料，停止後續判斷。

2. **篩出 `ERROR` 行**
   - 只保留包含 `ERROR` 的紀錄行。
   - 以這些行作為後續統計依據。

3. **判斷錯誤是否超過 10 筆**
   - 計算 `ERROR` 行數量。
   - 若**超過 10 筆**，走「是」分支。
   - 若**未超過 10 筆**，走「否」分支。

4. **若超過 10 筆：建立 Jira 問題單**
   - 建立一張 Jira issue，內容至少包含：
     - 錯誤摘要
     - `ERROR` 行數量
     - 相關 log 片段或時間範圍
     - 影響範圍（若可判定）

5. **通知值班工程師**
   - 將問題單資訊與錯誤摘要通知值班工程師。
   - 通知內容應清楚指出：錯誤已超過 10 筆、已建立 Jira 問題單。

6. **寫入每日摘要**
   - 不論是否超過 10 筆，最後都要寫入每日摘要。
   - 若超過 10 筆，摘要中要記錄：
     - 錯誤數量
     - Jira 單號
     - 已通知值班工程師
   - 若未超過 10 筆，摘要中要記錄：
     - 錯誤數量
     - 未達建立 Jira 與通知門檻

## 輸出要求

- 依流程完成後，回傳已執行的步驟與結果。
- 若資料不足以判斷是否超過 10 筆，明確說明缺少哪些 log 資訊。
