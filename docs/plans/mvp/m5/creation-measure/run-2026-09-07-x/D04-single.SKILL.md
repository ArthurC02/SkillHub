---
name: server-log-error-triage
description: Use when a process needs to review server logs, filter ERROR entries, decide whether the error count exceeds 10, and then either create a Jira issue and notify the on-duty engineer or record the result in a daily summary.
---

# 伺服器 log 錯誤處理流程

依照下列流程處理伺服器 log 中的錯誤事件。

## 流程步驟

1. **讀取伺服器 log 檔**
   - 取得指定時間範圍或指定來源的伺服器 log。
   - 若 log 檔無法取得，先記錄無法讀取的原因，並停止後續判斷。

2. **篩出 ERROR 行**
   - 從 log 中只保留包含 `ERROR` 的行。
   - 統計 ERROR 行的總數。

3. **判斷錯誤是否超過 10 筆**
   - 若 ERROR 筆數 **大於 10**，走「是」分支。
   - 若 ERROR 筆數 **不超過 10**，走「否」分支。

4. **若超過 10 筆：建立 Jira 問題單**
   - 建立一張 Jira issue，內容至少包含：
     - 錯誤摘要
     - ERROR 筆數
     - 相關 log 節錄或關鍵訊息
     - 發生時間範圍
     - 影響範圍或服務名稱（若可得）
   - 確保問題單可供後續追蹤與處理。

5. **通知值班工程師**
   - 將 Jira 問題單資訊與錯誤摘要通知值班工程師。
   - 通知內容應清楚說明：
     - 錯誤已超過門檻
     - Jira 單號或連結
     - 需要立即關注的重點

6. **寫入每日摘要**
   - 不論是否超過 10 筆，都要將結果寫入每日摘要。
   - 每日摘要至少包含：
     - 日期與時間
     - ERROR 筆數
     - 是否超過 10 筆
     - 若有建立 Jira，記錄單號
     - 若有通知值班工程師，記錄通知時間或方式

## 判斷規則

- **ERROR 筆數 > 10**：建立 Jira 問題單 → 通知值班工程師 → 寫入每日摘要。
- **ERROR 筆數 ≤ 10**：直接寫入每日摘要。

## 注意事項

- 不要把非 `ERROR` 行算入統計。
- 若 log 格式不一致，先以可辨識的 `ERROR` 行為準。
- 若無法確認筆數，應在摘要中註明資料不足或解析失敗。
