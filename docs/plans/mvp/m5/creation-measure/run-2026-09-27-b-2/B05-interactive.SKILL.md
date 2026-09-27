---
name: meeting-to-todo-list
description: 將會議記錄整理成待辦事項清單，當你需要把會議內容轉成可執行項目並附上負責人與期限時使用。
---

# Meeting to-do list

將使用者提供的會議記錄整理成待辦事項清單，並保留原文中的主要行動項目、負責人與期限。

## 你要做的事

1. 讀取使用者給的會議記錄。
2. 擷取每一個明顯的行動項目。
3. 為每一項輸出以下三欄：
   - 待辦內容
   - 負責人
   - 期限
4. 以繁體中文輸出成可直接閱讀的清單。
5. 若原文沒有明示負責人或期限，依上下文做最合理的推定，並在該欄標示為推定值。
6. 若原文同時出現多個行動項目，逐項列出，不要合併成模糊摘要。

## 輸出格式

優先使用這種格式：

- 待辦內容：...
  - 負責人：...
  - 期限：...

如果內容很多，也可以用表格，只要每一項都清楚包含三個欄位。

## 必須遵守的規則

when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently; never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 工作方式

- 如果使用者提供的會議記錄已經包含負責人與期限，就直接使用原文資訊。
- 如果使用者提供的會議記錄缺少負責人或期限，根據上下文做推定，但不要捏造不存在的事實。
- 如果原文沒有足夠資訊可推定，該欄寫 `not given`。
- 輸出時保持清楚、簡潔、可直接轉貼使用。
- 不要回問使用者要補資料；完成你能完成的版本。

## 範例

輸入：
今天會議討論三件事：第一，工程團隊週五前完成登入頁改版，Alice 負責；第二，行銷部門下週一前提交活動文案，Bob 與 Carol 協作；第三，財務部確認 10/15 前完成報銷流程更新，由 David 跟進。

輸出：
- 待辦內容：完成登入頁改版
  - 負責人：Alice
  - 期限：週五前
- 待辦內容：提交活動文案
  - 負責人：Bob、Carol
  - 期限：下週一前
- 待辦內容：完成報銷流程更新
  - 負責人：David
  - 期限：10/15 前