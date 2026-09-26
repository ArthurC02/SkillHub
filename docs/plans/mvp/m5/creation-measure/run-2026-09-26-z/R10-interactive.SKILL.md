---
name: slack-unreplied-questions-weekly
description: 每週從三個指定的 Slack 頻道找出未回覆的問題，並輸出成附連結的清單。當你要整理 Slack 頻道中的待回問題時使用。
---

# Slack 未回覆問題週報

你會收到一段使用者輸入，內容描述要從 Slack 頻道中找出未回覆的問題。你的任務是直接產出最終清單。

## 執行原則

1. 只使用輸入中提供的內容。
2. use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
3. deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
4. 只處理輸入明確指定的 3 個 Slack 頻道。
5. 只列出未回覆的問題。
6. 每筆結果都要附上可點開的連結；如果輸入沒有提供連結，就寫 `not given`。
7. 以每週整理的形式輸出；如果輸入沒有提供週期細節，就寫 `not given`。

## 工作步驟

1. 找出輸入中指定的 3 個 Slack 頻道。
2. 從這些頻道的內容中篩出問題訊息。
3. 判斷哪些問題是未回覆的。
4. 以清單列出每一則未回覆問題。
5. 為每筆結果附上對應連結；若未提供，標示 `not given`。

## 輸出格式

輸出一份清單，每筆至少包含：
- 頻道
- 問題內容
- 連結

如果輸入沒有足夠資訊判斷某欄位，寫 `not given`。

## 缺少資訊時

如果輸入沒有提供 3 個 Slack 頻道、問題內容或連結，仍然照樣輸出，並在對應欄位寫 `not given`。