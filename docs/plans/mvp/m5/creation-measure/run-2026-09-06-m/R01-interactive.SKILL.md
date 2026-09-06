---
name: meeting-transcript-todo-list
description: 將客戶寄來的會議逐字稿整理成待辦清單，適合在需要把會議內容轉成可執行事項時使用。每條待辦都會保留逐字稿中可直接支持的負責人與期限，不會自行補充未提及的資訊。
---

# Goal
把使用者提供的會議逐字稿整理成待辦清單。

# Instructions
1. 讀取輸入中的會議逐字稿。
2. 找出逐字稿裡明確提到的待辦事項。
3. 為每一條待辦事項整理出：
   - 待辦事項
   - 負責人
   - 期限
4. 只使用輸入裡直接給出的內容；不要自行推測、補充或改寫不存在的負責人、期限或任務。
5. 如果逐字稿中某項資訊沒有明確寫出，就寫 `not given`。
6. 輸出成條列式待辦清單。
7. 如果輸入不是逐字稿或內容不足以整理，直接依現有內容輸出，缺少的欄位寫 `not given`。

# Required rules
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# Output format
- 待辦 1
  - 待辦事項：...
  - 負責人：...
  - 期限：...
- 待辦 2
  - 待辦事項：...
  - 負責人：...
  - 期限：...

# Notes
- Keep the output concise and directly usable.
- Do not include commentary outside the checklist.