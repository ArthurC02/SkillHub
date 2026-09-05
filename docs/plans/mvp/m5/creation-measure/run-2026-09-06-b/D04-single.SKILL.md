---
name: error-log-triage-flow
description: 'Use when you need to follow a simple incident-handling flow from server logs: filter ERROR entries, decide whether the count exceeds 10, and then either create a Jira issue and notify the on-duty engineer or only write the daily summary.'
---

# 處理伺服器 log 的流程

依照圖中的流程執行，不要自行增加額外分支。

## 1. 讀取伺服器 log 檔
- 取得指定的伺服器 log 檔內容。
- 若 log 檔無法取得，先回報無法讀取，並停止後續判斷。

## 2. 篩出 ERROR 行
- 從 log 中只保留包含 `ERROR` 的行。
- 計算 ERROR 行的總數。

## 3. 判斷錯誤是否超過 10 筆
- 如果 ERROR 筆數 **大於 10**，走「是」分支。
- 如果 ERROR 筆數 **不大於 10**，走「否」分支。

## 4. 當錯誤超過 10 筆時
依序執行：
1. 建立 Jira 問題單。
2. 通知值班工程師。
3. 寫入每日摘要。

## 5. 當錯誤未超過 10 筆時
- 不建立 Jira 問題單。
- 不通知值班工程師。
- 直接寫入每日摘要。

## 6. 寫入每日摘要
- 在每日摘要中記錄：
  - log 檔來源或識別資訊
  - ERROR 行數量
  - 是否超過 10 筆
  - 若有執行，則記錄已建立 Jira 與已通知值班工程師

## 7. 輸出結果
- 回報已完成的步驟。
- 若有建立 Jira 或通知工程師，明確列出。
- 若未超過 10 筆，也要明確說明只完成了摘要記錄。
