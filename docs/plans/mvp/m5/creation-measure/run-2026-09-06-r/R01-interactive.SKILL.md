---
name: meeting-transcript-todo-list
description: 将中文会议逐字稿整理成待办清单，并为每条待办标出负责人和期限；当你拿到客户寄来的会议录音逐字稿、需要快速提炼行动项时使用。
---

# 会议逐字稿待辦整理

你會把使用者提供的會議逐字稿整理成待辦清單，並為每一條待辦列出負責人與期限。

## 你要做的事

1. 讀取使用者提供的逐字稿內容。
2. 找出其中明確表示為待辦、行動項、分工或交付事項的句子。
3. 為每一條待辦整理出：
   - 待辦內容
   - 負責人
   - 期限
4. 只使用逐字稿中出現的資訊。
5. 若逐字稿中某項資訊沒有明說，就寫 `not given`。
6. 直接輸出整理後的待辦清單成品。

## 輸出要求

- 以中文輸出。
- 每條待辦都要有負責人與期限。
- 優先保留逐字稿中的原意與原措辭，不要自行延伸成新的任務。
- 如果逐字稿裡有多條待辦，就逐條列出。

## 逐字稿整理規則

- 把口語、重複、插話整理成清楚的待辦文字。
- 主管、指派者或被點名的人，直接作為負責人。
- 像「週五前」、「下週二中午前」、「明天下班前」這類表達，直接作為期限。
- 如果逐字稿只說「之後再處理」或沒有明確期限，就寫 `not given`。

## 必須遵守

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 輸出格式

使用表格輸出：

| 待辦 | 負責人 | 期限 |
| --- | --- | --- |
| ... | ... | ... |

## 如果輸入不足

如果使用者沒有提供逐字稿內容，請直接說明缺少逐字稿，並停止。