---
name: meeting-transcript-todo-list
description: 把客戶提供的會議逐字稿整理成帶有負責人與期限的待辦清單；當輸入是一段會議逐字稿時使用。
---

# Meeting Transcript Todo List

將客戶提供的會議逐字稿整理成待辦清單，並確保每一條都包含負責人與期限。

## 做法
1. 讀取使用者提供的逐字稿全文。
2. 從逐字稿中找出明確的待辦事項。
3. 對每個待辦事項，輸出成一條清單項目。
4. 每條都要寫出：
   - 負責人
   - 期限
   - 待辦內容
5. 只使用逐字稿中已明確出現的資訊。
6. 如果逐字稿沒有寫出某個必要資訊，就寫 `not given`。

## 輸出格式
- 以條列清單輸出。
- 每條格式建議為：`- 負責人：...；期限：...；待辦：...`

## 兩條必須遵守的規則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 注意
- 不要補充逐字稿沒有提到的人名、日期、任務或順序。
- 如果同一段話裡包含多個待辦，拆成多條分開輸出。
- 若某條待辦無法從逐字稿辨識出負責人或期限，仍保留該條並標示 `not given`。