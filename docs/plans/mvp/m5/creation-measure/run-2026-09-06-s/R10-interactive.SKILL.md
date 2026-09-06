---
name: slack-unreplied-question-list
description: 每週從指定的三個 Slack 頻道中找出未回覆的問題，並整理成附連結的清單；當你拿到 Slack 訊息內容、匯出資料或可搜尋的訊息結果時使用。
---

# Slack 未回覆問題清單

請依照輸入中的 Slack 訊息內容，找出未回覆的問題，整理成清單並附上連結。

## 執行規則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- 只根據輸入裡可觀察到的訊息與回覆關係判定是否未回覆。
- 不要臆測誰算作回覆者、哪些話算問題、或連結是否存在；缺少資訊時寫 `not given`。
- 若輸入沒有提供足以判定的 Slack 訊息內容，輸出應直接說明缺少哪些內容，而不是自行補充。

## 步驟
1. 讀取輸入中的三個 Slack 頻道訊息與時間範圍。
2. 逐條找出看起來是問題、請求協助、或需要回應的訊息。
3. 檢查每條訊息是否有明確回覆，且回覆是否已解決或只是轉述。
4. 只保留可判定為未回覆的問題。
5. 為每條保留的問題輸出：頻道、問題摘要、連結。
6. 若某項欄位未提供，輸出 `not given`。
7. 以清楚可讀的清單格式交付最終結果。

## 輸出格式
- 頻道：...
- 問題摘要：...
- 連結：...

重複以上欄位，直到列完所有未回覆問題。

## 注意事項
- 只輸出最終清單，不要附加分析過程。
- 不要加入輸入沒有提供的時間、分類、優先級或結論。
- 如果輸入只是一組 Slack 匯出內容，就只處理這些內容；不要假設能存取其他來源。