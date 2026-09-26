---
name: server-log-error-escalation
description: Use this skill when a flowchart shows how to handle server log errors, especially when deciding whether repeated errors require escalation. It turns the diagram into step-by-step operational instructions.
---

# 伺服器 log 錯誤處理流程

依照流程圖執行以下步驟：

1. **讀取伺服器 log 檔**
   - 取得目標伺服器的 log 檔內容。
   - 確認讀取的是最新、完整的紀錄區間。

2. **篩出 ERROR 行**
   - 從 log 中只保留標記為 `ERROR` 的行。
   - 若需要，先整理成可統計的清單，方便後續計數。

3. **判斷錯誤是否超過 10 筆**
   - 計算 `ERROR` 行數。
   - 若錯誤數量 **超過 10 筆**，走「是」分支。
   - 若錯誤數量 **未超過 10 筆**，走「否」分支。

4. **若超過 10 筆：建立 Jira 問題單**
   - 建立一張 Jira issue，內容應包含：
     - 伺服器名稱或識別資訊
     - log 時間範圍
     - `ERROR` 行數
     - 主要錯誤摘要或代表性錯誤訊息
     - 影響範圍與初步觀察
   - 確保問題單可供後續追蹤與處理。

5. **通知值班工程師**
   - 將 Jira 問題單資訊通知值班工程師。
   - 通知內容至少包含：問題單連結、錯誤數量、嚴重性摘要、需要立即關注的原因。

6. **寫入每日摘要**
   - 不論是否超過 10 筆，最後都要寫入每日摘要。
   - 每日摘要至少記錄：
     - 伺服器或系統名稱
     - `ERROR` 行數
     - 是否已建立 Jira
     - 是否已通知值班工程師
     - 重要錯誤摘要

7. **若未超過 10 筆**
   - 不建立 Jira。
   - 不通知值班工程師。
   - 直接進入每日摘要記錄。

## 執行原則
- 以 `ERROR` 行數作為是否升級處理的判斷依據。
- 若 log 無法讀取或無法正確統計，先記錄問題並在每日摘要中註明異常。
- 保持紀錄一致，讓後續人員能追蹤處理結果。
