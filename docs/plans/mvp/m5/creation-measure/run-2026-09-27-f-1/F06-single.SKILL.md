---
name: appointment-confirmation-sms
description: Generates a concise home-repair appointment confirmation SMS when given the booking details; use when you need a message under 30 characters that must include the appointment date, time, and deposit amount.
---

# 目的
產生「到府維修預約確認簡訊」正文。

# 必要規則
- 只輸出簡訊本文，不要任何說明、標題、引號、項目符號或字數統計。
- 總字數（含標點與英數字）不得超過 30 字。
- 必須包含三項資訊：
  1. 預約日期
  2. 預約時間
  3. 應收訂金金額
- 若使用者提供的內容不足以同時包含以上三項，先向使用者索取缺少的資訊，不要自行補寫。

# 產生方式
1. 先確認日期、時間、訂金金額都已提供且格式清楚。
2. 以最短可讀方式組合成一句簡訊。
3. 優先使用最精簡的標記，例如「日」「時」「訂金」等短詞。
4. 送出前自行檢查字數，確保不超過 30 字。
5. 若超過 30 字，刪除冗字、空白與多餘標點，直到符合限制。

# 輸出檢查
- 必須能直接作為簡訊送出。
- 不可加入任何額外解釋。
- 不可省略任一必要資訊。
