---
name: server-log-error-daily-summary
description: 將伺服器 log 轉成中文每日摘要；當 ERROR 數量超過 10 筆時，額外產出 Jira 問題單與值班工程師通知內容。適合在收到 log 內容、需要快速整理錯誤事件與判斷是否需升級處理時使用。
---

# 目的
將使用者提供的伺服器 log 轉成中文每日摘要，並依據 ERROR 數量是否超過 10 筆，決定是否同時產出 Jira 問題單與通知值班工程師的內容。

兩條規則必須原樣遵守：
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# 執行步驟
1. 讀取伺服器 log 檔。
2. 篩出 ERROR 行。
3. 判斷錯誤是否超過 10 筆。
4. 建立 Jira 問題單。
5. 通知值班工程師。
6. 寫入每日摘要。

# 產出規則
- 只使用輸入中實際出現的內容。
- 若任何必要資訊在輸入中沒有提供，寫 `not given`。
- 不要補日期、姓名、系統名稱、工單內容、收件人或其他未提供資訊。
- 輸出必須是完成品本身，不要解釋流程或回覆詢問。
- 以中文輸出。

# 節點細則
## 讀取伺服器 log 檔
直接處理使用者提供的 log 文字。

## 篩出 ERROR 行
只保留包含 `ERROR` 的行；其他行不納入錯誤清單，但仍可在摘要中被提及為非錯誤事件，前提是輸入中有明確內容。

## 建立 Jira 問題單
只有在 ERROR 總數超過 10 筆時才執行。
若輸入未提供 Jira 標題、描述、專案或其他必要欄位，對應位置寫 `not given`。

## 通知值班工程師
只有在 ERROR 總數超過 10 筆時才執行。
若輸入未提供值班工程師姓名、聯絡方式或通知內容，對應位置寫 `not given`。

## 寫入每日摘要
一定要輸出每日摘要。
摘要只能整理輸入中存在的事件、數量與明確文字；缺少者寫 `not given`。

# 輸出格式
用清楚的小標題輸出以下內容：
- ERROR 行
- 是否超過 10 筆
- Jira 問題單
- 通知值班工程師
- 每日摘要

若某一項在當前條件下不適用，仍要保留該小標題，並在內容寫 `not given` 或 `不適用`；其中 `不適用` 只用於因 ERROR 數量不超過 10 筆而不執行的 Jira 與通知步驟。

# 注意
- 不要捏造任何外部系統結果。
- 不要假設你真的已建立 Jira 或真的已送出通知；只輸出根據輸入整理出的完成內容。