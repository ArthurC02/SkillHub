---
name: slack-unanswered-question-weekly-list
description: Extract unanswered questions from three specified Slack channels each week and list them with links. Use when you have Slack message content or exports from those channels and need a readable weekly triage list.
---

# Slack 未回覆問題週清單

## 何時使用
當你收到三個指定 Slack 頻道的訊息內容或匯出內容，並且要找出其中未回覆的問題、整理成每週清單時使用。

## 你要做的事
1. 讀入使用者提供的 Slack 訊息內容。
2. 只處理三個指定頻道的內容。
3. 找出屬於「問題」且可判定為「未回覆」的訊息。
4. 為每一筆保留頻道名稱、問題內容與訊息連結。
5. 輸出一份每週清單。

## 必須遵守
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 處理原則
- 只列出未回覆的問題。
- 只涵蓋三個指定頻道。
- 不推測未提供的回覆狀態。
- 如果輸入沒有足夠資訊判定某筆訊息是否未回覆，寫 `not given`。
- 如果輸入沒有提供訊息連結，寫 `not given`。
- 如果輸入沒有明確指出三個頻道是哪些，寫 `not given`。

## 輸出格式
以清單輸出，每筆一行或一個條目，至少包含：
- 頻道名稱
- 問題內容
- 訊息連結

## 範例判定方式
- 看到明確的回覆訊號時，不列入清單。
- 看到只有提問、沒有回覆訊號時，列入清單。
- 若同一題後面已有回覆訊號，視為已回覆，不列不入。

## 最後檢查
輸出前確認：
- 只有三個指定頻道的內容
- 只有未回覆的問題
- 每筆都有頻道名稱、問題內容與訊息連結
- 產出的是每週清單，而不是分析說明