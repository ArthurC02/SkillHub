---
name: server-log-error-triage
description: 'Use when you need to process a server log, filter ERROR entries, and decide whether the incident warrants escalation. It follows a simple flow: count errors, create a Jira issue and notify the on-call engineer if errors exceed 10, then write the daily summary.'
---

# 伺服器 log 錯誤處理流程

依照下列流程處理伺服器 log：

1. **讀取伺服器 log 檔**
   - 取得要檢查的 log 檔內容。
   - 若 log 檔無法取得，先記錄無法處理的原因，並停止後續步驟。

2. **篩出 `ERROR` 行**
   - 從 log 中只保留包含 `ERROR` 的行。
   - 統計 `ERROR` 行數，作為後續判斷依據。

3. **判斷錯誤是否超過 10 筆**
   - 若 `ERROR` 行數 **大於 10**，走「是」分支。
   - 若 `ERROR` 行數 **不超過 10**，走「否」分支。

4. **若超過 10 筆：建立 Jira 問題單**
   - 建立一張 Jira issue，內容至少包含：
     - log 檔名稱或來源
     - `ERROR` 行數
     - 主要錯誤摘要或代表性錯誤訊息
     - 發生時間範圍（若可從 log 推得）
   - 讓問題單能支持後續追查與處理。

5. **通知值班工程師**
   - 將 Jira 問題單資訊與錯誤摘要通知值班工程師。
   - 通知內容應清楚說明：錯誤數量已超過門檻，需要優先處理。

6. **寫入每日摘要**
   - 不論是否超過 10 筆，最後都要寫入每日摘要。
   - 摘要至少包含：
     - log 檔來源
     - `ERROR` 行數
     - 是否已建立 Jira
     - 是否已通知值班工程師
     - 重要錯誤摘要

## 判斷規則
- `ERROR` 行數 > 10：建立 Jira、通知值班工程師、再寫每日摘要。
- `ERROR` 行數 ≤ 10：直接寫每日摘要。

## 輸出重點
- 保持錯誤數量與處理結果一致。
- 每日摘要要能讓後續讀者快速知道是否需要追蹤。
