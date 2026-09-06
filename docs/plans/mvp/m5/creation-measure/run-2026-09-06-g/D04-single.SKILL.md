---
name: error-log-triage-flow
description: Use when you need to process a server log, filter ERROR entries, and decide whether the incident warrants escalation based on volume. It turns the flowchart into a step-by-step triage procedure for creating a Jira issue, notifying the on-call engineer, and writing the daily summary.
---

# 伺服器 log 錯誤處理流程

依照下列流程處理伺服器 log：

1. **讀取伺服器 log 檔**
   - 取得要檢查的 log 檔內容。
   - 若有多個檔案，先確認要處理的範圍與時間區間。

2. **篩出 ERROR 行**
   - 從 log 中只保留包含 `ERROR` 的行。
   - 記錄錯誤行數與主要錯誤內容。

3. **判斷錯誤是否超過 10 筆**
   - 統計 `ERROR` 行數。
   - 若 **超過 10 筆**，走「是」分支。
   - 若 **10 筆或以下**，走「否」分支。

4. **若超過 10 筆：建立 Jira 問題單**
   - 建立一張 Jira issue。
   - 在內容中包含：
     - log 檔名稱或來源
     - 錯誤發生時間範圍
     - `ERROR` 行數
     - 代表性錯誤訊息
     - 影響範圍與初步判斷

5. **通知值班工程師**
   - 將 Jira 問題單資訊與錯誤摘要通知值班工程師。
   - 通知內容要能讓對方快速判斷是否需要立即處理。

6. **寫入每日摘要**
   - 不論是否超過 10 筆，最後都要寫入每日摘要。
   - 摘要至少包含：
     - 今日檢查的 log 來源
     - `ERROR` 行數
     - 是否建立 Jira
     - 是否通知值班工程師
     - 重要觀察或後續建議

## 分支規則

- **ERROR 超過 10 筆**：先建立 Jira，再通知值班工程師，最後寫入每日摘要。
- **ERROR 不超過 10 筆**：直接寫入每日摘要。

## 執行原則

- 只根據 log 中可驗證的內容做判斷。
- 若錯誤訊息重複，仍以實際行數計算是否超過 10 筆。
- 若資訊不足以建立完整 Jira，先整理可用資訊，再補齊缺少的欄位。
