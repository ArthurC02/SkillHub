---
name: slack-unreplied-questions-weekly-list
description: 每週從 Slack 的三個指定頻道找出未回覆的問題，並輸出成附連結的清單；當你要整理指定頻道中的待回覆提問時使用。
---

# Slack 未回覆問題週清單

你會收到一段使用者提供的 Slack 材料。你的任務是從中找出三個指定頻道內、在該週範圍中的未回覆問題，整理成清單並附上連結。

## 執行原則

- 只使用輸入中提供的內容。
- 只處理輸入裡明確給出的三個頻道與該週材料。
- 輸出成清單，每列對應一則未回覆的問題。
- 每列都要包含問題內容與可點擊連結。
- 若輸入沒有提供可用的頻道名稱、訊息內容、週範圍或連結資訊，就在輸出中寫 `not given`。
- 只輸出成品，不要描述規則、流程或請求更多存取權限。

## 判定與整理步驟

1. 找出輸入中提到的三個頻道。
2. 檢視這三個頻道內的訊息。
3. 判斷哪些訊息是問題，且在輸入材料中沒有對應回覆。
4. 將符合條件的項目整理成清單。
5. 為每一項附上輸入中可得的連結；若沒有連結，寫 `not given`。
6. 以「每週」作為本次整理的範圍，只處理輸入中對應那一週的材料。

## 輸出格式

使用下列格式輸出：

- 頻道：<頻道名稱>
  - 問題：<問題內容>
  - 連結：<連結或 not given>

## 例外情況

- 如果輸入未提供足夠材料來判定是否未回覆，就只根據現有材料輸出，並把缺少的欄位寫成 `not given`。
- 不要補寫輸入沒有給的頻道、日期、使用者名稱或回覆關係。

## 必須遵守的兩條規則

use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.