---
name: server-log-error-triage
description: 'Use this skill when you need to triage server log files for ERROR entries and decide whether the issue escalates into a Jira ticket and on-call notification. It follows a simple flow: read logs, filter errors, check whether the error count exceeds 10, then either create a Jira issue and notify the on-duty engineer or just write the daily summary.'
---

# 伺服器 log 錯誤處理流程

依照下列流程處理伺服器 log：

1. **讀取伺服器 log 檔**
   - 取得當日或指定期間的伺服器 log。
   - 確認檔案可讀，並保留原始內容以便追查。

2. **篩出 `ERROR` 行**
   - 從 log 中只保留包含 `ERROR` 的行。
   - 若需要統計，將每一筆錯誤行分開計數。

3. **判斷錯誤是否超過 10 筆**
   - 計算 `ERROR` 行數量。
   - 若**超過 10 筆**，走「是」分支。
   - 若**未超過 10 筆**，走「否」分支。

4. **若超過 10 筆：建立 Jira 問題單**
   - 建立一張 Jira issue，內容至少包含：
     - log 時間範圍
     - `ERROR` 行數量
     - 代表性錯誤訊息
     - 相關服務或主機名稱（若可從 log 判斷）
   - 標註這是需要進一步處理的異常事件。

5. **通知值班工程師**
   - 將 Jira 問題單資訊與錯誤摘要通知值班工程師。
   - 通知內容應簡潔，包含：
     - 問題單連結或編號
     - 錯誤數量
     - 是否需要立即處理

6. **寫入每日摘要**
   - 不論錯誤是否超過 10 筆，都要寫入每日摘要。
   - 摘要至少包含：
     - log 日期/區間
     - `ERROR` 數量
     - 是否已建立 Jira
     - 是否已通知值班工程師
     - 重要觀察或備註

## 執行原則

- 只根據 log 內容與錯誤數量做判斷，不要自行臆測原因。
- 若 log 不完整或無法讀取，先記錄問題並在摘要中註明。
- 若 Jira 或通知系統不可用，保留待辦資訊，並在每日摘要中明確標示未完成項目。
