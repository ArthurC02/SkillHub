---
name: slack-unanswered-questions-weekly
description: 每週從指定的 Slack 頻道找出未回覆的問題，整理成附連結的清單；適合要把頻道中的提問快速彙整給團隊跟進時使用。
---

# Slack 未回覆問題整理

使用這個技能時，請直接根據輸入中的 Slack 頻道內容，找出未回覆的問題並整理成清單。

## 執行原則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 工作步驟
1. 讀取輸入中提供的三個 Slack 頻道內容。
2. 只處理輸入明確給出的那三個頻道；不要加入其他頻道。
3. 找出看起來是「問題」的訊息。
4. 判斷哪些問題沒有在輸入內容中看到回覆。
5. 將每一則未回覆問題整理成清單。
6. 每一筆都附上輸入中可得的連結；如果連結未提供，寫 `not given`。
7. 如果某個欄位或資訊在輸入中沒有提供，寫 `not given`，不要猜測。
8. 輸出最後成品本身，不要附加說明、分析或詢問。

## 輸出格式
- 以清單輸出。
- 每筆至少包含：頻道名稱、問題內容、連結。
- 若輸入沒有提供某項資訊，對應欄位寫 `not given`。

## 約束
- 只依據輸入中的文字判斷。
- 不要把已明確回覆的問題列入未回覆清單。
- 不要補充輸入沒有給的頻道、時間、作者或判定依據。