---
name: server-log-error-triage
description: Use this skill when a flowchart shows how to handle server log errors, especially when the task is to turn the diagram into step-by-step operational instructions. It reads the process from the image and converts it into an actionable checklist.
---

# 伺服器 log 錯誤處理流程

依照圖中的流程執行，不要自行補充未出現在圖上的步驟。

## 流程

1. **讀取伺服器 log 檔**
   - 先取得並檢視伺服器的 log 檔內容。

2. **篩出 ERROR 行**
   - 從 log 中找出所有包含 `ERROR` 的行。
   - 只保留錯誤相關紀錄，作為後續判斷依據。

3. **判斷錯誤是否超過 10 筆**
   - 統計 `ERROR` 行數。
   - 若**超過 10 筆**，走「是」分支。
   - 若**未超過 10 筆**，走「否」分支。

4. **若超過 10 筆：建立 Jira 問題單**
   - 建立 Jira issue，記錄錯誤狀況。
   - 問題單內容應包含：錯誤摘要、發生時間範圍、相關 log 片段與影響說明。

5. **若超過 10 筆：通知值班工程師**
   - 在建立問題單後，通知值班工程師處理。
   - 通知內容應包含 Jira 編號與錯誤重點。

6. **寫入每日摘要**
   - 不論是否超過 10 筆，最後都要寫入每日摘要。
   - 若有建立 Jira 與通知值班工程師，也一併在摘要中記錄。

## 執行原則

- 這個流程的關鍵判斷是「ERROR 行是否超過 10 筆」。
- 圖中沒有顯示其他分支動作；若未超過 10 筆，直接進入每日摘要。
- 若無法取得 log 檔、無法建立 Jira，或無法通知值班工程師，應在摘要中記錄阻礙原因。
