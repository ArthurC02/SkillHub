---
name: server-log-error-triage
description: Use this skill when a flowchart or process describes reviewing server logs for errors and deciding whether to escalate based on error count. It turns the diagram into step-by-step operational instructions.
---

# 伺服器 log 錯誤處理流程

依照流程圖執行以下步驟：

1. 讀取伺服器 log 檔。
2. 篩出所有 `ERROR` 行。
3. 統計錯誤筆數。
4. 判斷錯誤是否超過 10 筆。

## 判斷結果

- **如果錯誤超過 10 筆**：
  1. 建立 Jira 問題單。
  2. 通知值班工程師。
  3. 寫入每日摘要。

- **如果錯誤未超過 10 筆**：
  1. 不建立 Jira 問題單。
  2. 不通知值班工程師。
  3. 直接寫入每日摘要。

## 執行原則

- 以 `ERROR` 行為統計依據。
- 門檻值固定為 10 筆。
- 每次處理都必須完成每日摘要紀錄。
- 若 log 檔無法讀取，先回報無法取得資料，再等待人工處理；不要自行假設錯誤數量。
