---
name: server-log-error-triage
description: Use when the task is to process server logs, filter ERROR entries, and decide whether the issue escalation threshold has been reached. It guides the agent through the flow of creating a Jira issue, notifying the on-call engineer, and writing a daily summary when errors do or do not exceed 10.
---

# 伺服器 log 錯誤處理流程

依照流程圖執行，順序不可省略。

## 1. 讀取伺服器 log 檔
- 取得指定的伺服器 log 檔內容。
- 若 log 檔不存在、無法讀取或內容不完整，先回報無法處理，並說明缺少的資訊。

## 2. 篩出 ERROR 行
- 從 log 中只保留包含 `ERROR` 的行。
- 計算 ERROR 行的總數。
- 若需要後續追蹤，整理出每一筆 ERROR 的時間、來源與簡要訊息。

## 3. 判斷錯誤是否超過 10 筆
- 比對 ERROR 總數是否大於 10。
- 這個判斷只看數量，不做其他條件推論。

## 4. 若錯誤超過 10 筆
- 建立 Jira 問題單。
- 問題單內容至少包含：
  - log 檔名稱或來源
  - ERROR 總數
  - 主要錯誤摘要
  - 影響時間範圍
  - 任何可辨識的重複模式
- 接著通知值班工程師。
- 通知內容要簡潔明確，包含：
  - 已超過 10 筆 ERROR
  - Jira 問題單資訊
  - 需要立即關注的原因

## 5. 若錯誤未超過 10 筆
- 不建立 Jira 問題單。
- 不通知值班工程師，除非其他明確要求另有規定。

## 6. 寫入每日摘要
- 不論是否超過 10 筆，最後都要寫入每日摘要。
- 每日摘要至少包含：
  - log 檔來源
  - ERROR 總數
  - 是否超過 10 筆
  - 若有建立 Jira，附上 Jira 編號或連結
  - 若有通知值班工程師，註明已通知

## 7. 輸出結果
- 回傳已完成的處理結果。
- 若有建立 Jira 與通知工程師，清楚列出。
- 若未超過門檻，清楚列出僅完成篩選與每日摘要。
