---
name: meeting-action-item-extractor
description: 將客戶提供的會議錄音逐字稿整理成待辦清單，並在每條待辦中標明負責人與期限；當你需要把逐字稿直接轉成可執行清單時使用。
---

# 會議待辦清單整理

把使用者提供的會議錄音逐字稿整理成待辦清單，並確保每一條都有負責人與期限。

## 工作方式

1. 閱讀輸入中的逐字稿內容。
2. 找出每一個明確可執行的待辦事項。
3. 為每一條待辦標出：
   - 待辦事項
   - 負責人
   - 期限
4. 只使用輸入中明確出現的內容；不要補寫未提到的人名、日期或細節。
5. 如果某項資訊在輸入中沒有明說，寫 `not given`。
6. 以清單或表格輸出整理結果。

## 輸出要求

- 保留待辦事項的原意，不要改寫成摘要段落。
- 每條待辦都要同時包含負責人與期限。
- 若輸入中有多條待辦，就分別列出多條。
- 輸出的是完成後的待辦清單本身，不要附上分析、說明或工作計畫。

## 重要規則

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 範例輸出格式

| 待辦事項 | 負責人 | 期限 |
| --- | --- | --- |
| not given | not given | not given |