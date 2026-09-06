---
name: meeting-transcript-todo-list
description: 將會議逐字稿整理成待辦清單，並為每一條待辦補上負責人與期限。適合在客戶提供會議錄音逐字稿、需要快速整理成可執行事項時使用。
---

# 目的
將使用者提供的會議逐字稿整理成待辦清單，且每一條待辦都要有負責人與期限。

# 執行方式
1. 讀取使用者給的逐字稿。
2. 找出逐字稿中明確表示的待辦事項。
3. 對每一條待辦，保留或整理出：
   - 待辦內容
   - 負責人
   - 期限
4. 以清單形式輸出結果。
5. 只根據輸入逐字稿整理，不加入逐字稿沒有提供的事項。

# 輸出要求
- 每一條待辦都必須包含負責人與期限。
- 輸出要是清單型格式，不要寫成段落敘述。
- 如果逐字稿中有多個待辦，就逐條列出。
- 如果逐字稿中某項資訊沒有明說，寫「not given」。
- 如果輸入不是逐字稿或內容不足以整理成待辦，直接說明缺少必要資訊。

# 必須遵守的兩條規則
use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# 輸出格式
用條列清單輸出，每一條至少包含：
- 待辦
- 負責人
- 期限

# 範例處理原則
- 若逐字稿直接寫出「我來」「由我跟進」「負責」等指向某人的內容，就把那個人整理成負責人。
- 若期限明確出現，就原樣整理成期限。
- 若期限未明說，就寫「not given」。
- 若待辦內容可從逐字稿明確整理出來，就只保留該內容，不延伸補充。