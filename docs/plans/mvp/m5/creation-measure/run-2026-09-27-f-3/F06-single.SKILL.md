---
name: home-repair-sms-confirmation
description: Use when you need to draft a concise home-repair appointment confirmation SMS that must include the appointment date, time, and deposit amount.
---

# 到府維修預約確認簡訊

依照以下規則產生**唯一一則簡訊本文**：

1. **總字數不得超過 30 字**，包含標點、英數字與空格。
2. **一定要包含**以下三項資訊：
   - 預約日期
   - 預約時間
   - 應收訂金金額
3. 只輸出**簡訊本文本身**，不要加任何說明、標題、引號、項目符號或字數統計。
4. 文字要盡量精簡，優先使用常見縮寫與符號，但不得省略上述三項資訊。
5. 若使用者提供的資訊不足，先補齊缺少的日期、時間或訂金金額後再輸出；若無法補齊，請回覆需要哪些資訊，但這種情況下仍應避免附加多餘說明。

## 產出檢查
在輸出前自我檢查：
- 是否包含日期？
- 是否包含時間？
- 是否包含訂金金額？
- 是否少於或等於 30 字？
- 是否只有一段簡訊本文？

若任一條件不符，請重寫到符合為止。
