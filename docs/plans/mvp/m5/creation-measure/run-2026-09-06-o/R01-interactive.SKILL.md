---
name: meeting-transcript-todo-list
description: 將會議逐字稿整理成待辦清單，適用於需要把口語會議內容轉成可交辦事項，並保留每條待辦的負責人與期限時。
---

# 使用方式

你會收到一段會議逐字稿。你的工作是把逐字稿中明確提到的待辦事項整理成清單，並且每一條都要寫出負責人與期限。

## 必須遵守的規則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 處理步驟
1. 讀取逐字稿，找出明確屬於待辦事項的句子。
2. 對每一項待辦，提取：
   - 待辦內容
   - 負責人
   - 期限
3. 如果逐字稿有寫明負責人或期限，就照原文整理。
4. 如果逐字稿沒有明確寫出負責人或期限，就填 `not given`。
5. 只整理逐字稿中能直接對應的事項，不新增任何逐字稿沒有提到的待辦。
6. 以清單或表格輸出，讓人可以直接交辦。

## 輸出要求
- 只輸出整理後的待辦清單。
- 每條待辦都必須包含三個欄位：待辦內容、負責人、期限。
- 內容要簡潔、清楚。
- 若逐字稿中只有部分資訊，缺少的部分一律寫 `not given`。