---
name: slack-unanswered-question-roundup
description: 每週從使用者指定的 3 個 Slack 頻道中找出未回覆的問題，整理成附連結的清單；當你要把 Slack 訊息彙整成待回覆事項時使用。
---

# Slack 未回覆問題每週彙整

你要把使用者提供的 Slack 資料整理成「未回覆問題」清單，並附上每筆的 Slack 連結。

## 先套用這兩條規則

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 你要做的事

1. 讀取使用者提供的 3 個 Slack 頻道內容。
2. 只找出未回覆的問題。
3. 為每一筆列出：
   - 頻道
   - 提問者
   - 問題摘要
   - Slack 連結
   - 判定為未回覆的原因或結果
4. 只輸出未回覆的問題，不要加入已回覆項目。

## 判定原則

- 如果同一串對話已經有回覆，就不要列入結果。
- 凡是被輸出的每一筆都必須附上使用者提供的、可直接開啟該訊息的實際 Slack 連結；不要輸出任何沒有這種連結的項目，也不要以任何占位文字填充連結欄位。
- 只使用輸入中明確提供的 Slack 訊息與連結；不要補寫不存在的內容。

## 輸出格式

用清單輸出，每筆一行，格式如下：

- 頻道：...
  - 提問者：...
  - 問題摘要：...
  - Slack 連結：...
  - 未回覆判定：...

## 注意事項

- 只處理使用者給的 3 個頻道。
- 不要編造回覆、連結或頻道資訊。
- 不要把已回覆的串放進結果。