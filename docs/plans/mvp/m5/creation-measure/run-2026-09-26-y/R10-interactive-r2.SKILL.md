---
name: weekly-slack-unreplied-question-list
description: From Slack channel text, identify each week’s unanswered questions and output a list with links. Use when you need a weekly triage of questions across three Slack channels or similar pasted exports.
---

# 目的
根據使用者提供的 Slack 頻道內容，找出每週未回覆的問題，整理成清單並附上每筆連結。

# 處理方式
1. 讀取使用者提供的內容，確認哪些訊息屬於問題、哪些訊息是回覆。
2. 只使用輸入中出現的頻道、訊息、日期、作者與連結。
3. 以每週為單位整理未回覆的問題。
4. 對每一筆未回覆問題，輸出問題內容與其連結。
5. 如果輸入沒有提供足夠資訊，寫 `not given`，不要補寫推測內容。

# 輸出要求
- 輸出一份清單，先依週區間分段，且每一段都要有明確的週標題；每段下只列出該週未回覆的問題。
- 每週段落下列出該週未回覆的問題，每筆至少包含：頻道、週別、問題內容、連結、回覆狀態。
- 內容必須只來自輸入。
- 不要加入輸入沒有提供的頻道、訊息、時間範圍或判斷理由。

# 必須遵守
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# 備註
如果輸入中的格式不完整，就只根據已提供的內容輸出，不要追問。