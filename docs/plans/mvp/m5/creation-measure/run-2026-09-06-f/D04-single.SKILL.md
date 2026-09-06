---
name: server-log-error-triage
description: Use when you need to triage a server log by filtering ERROR entries, deciding whether the error count exceeds 10, and then either opening a Jira issue and notifying the on-duty engineer or only recording the result in the daily summary.
---

# 伺服器 log 錯誤處理流程

依照下列流程處理伺服器 log：

1. **讀取伺服器 log 檔**
   - 取得要檢查的 log 檔內容。
   - 若有多個檔案，先確認要處理的範圍，再逐一讀取。

2. **篩出 ERROR 行**
   - 只保留包含 `ERROR` 的 log 行。
   - 統計篩出的 ERROR 行數。

3. **判斷錯誤是否超過 10 筆**
   - 若 **超過 10 筆**，進入升級處理。
   - 若 **不超過 10 筆**，直接進入每日摘要。

4. **若超過 10 筆：建立 Jira 問題單**
   - 彙整錯誤摘要：包含錯誤數量、主要錯誤訊息、發生時間範圍、受影響服務或主機。
   - 建立 Jira issue，標題要能清楚表達「伺服器 ERROR 超過 10 筆」與相關服務名稱。
   - 內容要附上可追查的 log 摘要與必要上下文。

5. **通知值班工程師**
   - 將 Jira 問題單資訊與錯誤摘要通知值班工程師。
   - 通知內容應包含：錯誤數量、Jira 連結或編號、是否持續發生、建議優先處理。

6. **寫入每日摘要**
   - 不論是否超過 10 筆，最後都要寫入每日摘要。
   - 每日摘要至少包含：
     - 檢查的 log 檔名稱或來源
     - ERROR 行數
     - 是否超過 10 筆
     - 若有建立 Jira，記錄 Jira 編號
     - 若有通知值班工程師，記錄通知時間或方式

## 執行原則
- 只根據 `ERROR` 行做判斷，不要把其他等級混入統計。
- 若錯誤訊息重複，仍以實際 ERROR 行數計算，除非後續摘要需要另外整理重複項。
- 若 log 內容不足以判斷是否超過 10 筆，先補齊資料再繼續，不要猜測。
