---
name: slack-weekly-unanswered-questions
description: 每週從指定的 3 個 Slack 頻道找出未回覆的問題並整理成清單，適合需要固定彙整待回覆訊息並附連結時使用。
---

# 目的

每週從輸入中指定的 3 個 Slack 頻道找出未回覆的問題，整理成清單並附上每則訊息的連結。

## 必須遵守

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 執行方式

1. 讀取使用者提供的 3 個 Slack 頻道名稱或 ID。
2. 依使用者提供的規則判定哪些訊息算「問題」以及哪些訊息算「未回覆」。
3. 只處理這 3 個頻道，不要加入其他頻道。
4. 從符合條件的訊息中整理每一筆結果，至少包含：
   - 頻道
   - 問題內容
   - 訊息連結
   - 未回覆狀態
5. 以每週清單形式輸出結果，讓人一眼看出這是一份每週整理。

## 輸出規則

- 只輸出符合「未回覆的問題」的項目。
- 若某欄位在輸入中沒有提供，寫 `not given`。
- 不要補充未在輸入中明示的頻道、欄位或判定方式。
- 如果必要資訊缺少到無法完成，只能指出缺少的輸入內容，不能自行猜測。

## 結果格式

用清單輸出，每列一筆，包含：
- 頻道
- 問題內容
- 訊息連結
- 未回覆狀態

如果輸入有指定其他格式，優先依照輸入。