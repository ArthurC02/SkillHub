---
name: error-log-triage-flow
description: Use when the task is to follow a log-triage flowchart for server errors, especially deciding whether to escalate after counting ERROR lines in a server log. It turns the diagram into step-by-step instructions for handling the log, creating a Jira issue, notifying the on-call engineer, or only writing the daily summary.
---

# 伺服器 log 錯誤處理流程

依照流程圖執行，不要自行改變判斷順序。

## 步驟

1. **讀取伺服器 log 檔**
   - 取得要檢查的伺服器 log。
   - 以同一個檢查範圍內的內容為準，避免混用不同時間區段或不同來源的 log。

2. **篩出 `ERROR` 行**
   - 找出所有包含 `ERROR` 的紀錄行。
   - 計算 `ERROR` 行的總數。

3. **判斷錯誤是否超過 10 筆**
   - 若 `ERROR` 行數 **大於 10**，走「是」分支。
   - 若 `ERROR` 行數 **不大於 10**，走「否」分支。

4. **若超過 10 筆：建立 Jira 問題單**
   - 建立一張 Jira issue，內容應包含：
     - log 檔名稱或來源
     - 檢查時間範圍
     - `ERROR` 行數
     - 主要錯誤摘要或代表性錯誤訊息
   - 目標是讓後續處理者能快速理解問題。

5. **通知值班工程師**
   - 在建立 Jira 後，通知值班工程師。
   - 通知內容應包含 Jira 連結或編號，以及錯誤摘要。

6. **寫入每日摘要**
   - 不論是否超過 10 筆，最後都要寫入每日摘要。
   - 若有建立 Jira 與通知值班工程師，摘要中要記錄這些動作。
   - 若未超過 10 筆，摘要中只需記錄已檢查 log、`ERROR` 行數，以及未觸發升級。

## 分支規則

- **`ERROR` > 10**：讀取 log → 篩出 `ERROR` 行 → 判斷超過 10 → 建立 Jira 問題單 → 通知值班工程師 → 寫入每日摘要
- **`ERROR` ≤ 10**：讀取 log → 篩出 `ERROR` 行 → 判斷未超過 10 → 直接寫入每日摘要

## 注意事項

- 不要在未完成 `ERROR` 行統計前建立 Jira。
- 不要跳過每日摘要。
- 若 log 內容不足以判斷是否超過 10 筆，先補齊必要資訊再繼續。
