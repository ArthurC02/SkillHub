---
name: server-log-error-triage
description: Use when you need to triage a server log by checking for ERROR entries, deciding whether the error count exceeds 10, and then either escalating with a Jira issue and on-call notification or recording the result in a daily summary.
---

# 伺服器 log 錯誤處理流程

依照下列流程處理伺服器 log：

1. **讀取伺服器 log 檔**
   - 取得目標伺服器的 log 檔內容。
   - 確認分析範圍與時間區間一致，避免混入其他批次或其他日期的紀錄。

2. **篩出 ERROR 行**
   - 只保留包含 `ERROR` 的 log 行。
   - 統計 ERROR 行的數量，並保留必要的上下文以便後續追查。

3. **判斷錯誤是否超過 10 筆**
   - 若 ERROR 筆數 **超過 10 筆**，視為需要升級處理。
   - 若 ERROR 筆數 **未超過 10 筆**，直接進入每日摘要。

4. **若超過 10 筆：建立 Jira 問題單**
   - 建立一張 Jira issue，內容至少包含：
     - 伺服器名稱或服務名稱
     - log 檔來源與時間範圍
     - ERROR 筆數
     - 主要錯誤訊息與重現線索
     - 已觀察到的影響範圍
   - 確保問題單可供後續追蹤與分派。

5. **通知值班工程師**
   - 將 Jira 問題單與關鍵摘要通知值班工程師。
   - 通知內容應簡潔明確，包含嚴重程度、錯誤數量與需要立即查看的重點。

6. **寫入每日摘要**
   - 不論是否超過 10 筆，最後都要寫入每日摘要。
   - 每日摘要至少記錄：
     - 日期與服務名稱
     - ERROR 筆數
     - 是否已建立 Jira
     - 是否已通知值班工程師
     - 重要觀察或後續建議

## 執行原則
- 只根據 `ERROR` 行做判斷，不要把其他等級的 log 混入統計。
- 若資訊不足以判斷是否超過 10 筆，先補齊 log 範圍與來源，再繼續。
- 若建立 Jira 或通知值班工程師需要外部系統權限，而目前沒有可用權限，則明確記錄缺少的資訊，並先完成可做的分析與每日摘要。
