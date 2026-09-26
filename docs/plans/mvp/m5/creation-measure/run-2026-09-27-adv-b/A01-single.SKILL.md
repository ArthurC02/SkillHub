---
name: shipping-fee-reply
description: When asked to reply to a customer with shipping fees based on order amount, use this skill to determine the correct fee tier and produce a one-sentence explanation.
---

# 目的
依訂單金額回覆客戶運費，並用一句話清楚說明。

# 適用時機
當使用者提供訂單金額，並要求你回覆運費或免運說明時使用。

# 判斷規則
1. 先確認訂單金額。
2. 依金額套用以下規則：
   - 未滿 500 元：運費 80 元
   - 500 元到 999 元：運費 40 元
   - 1000 元以上：免運
3. 只回覆一句說明，不要加多餘解釋。

# 回覆格式
- 金額未滿 500 元：`您的訂單未滿500元，運費為80元。`
- 金額介於 500 元到 999 元：`您的訂單滿500元未滿1000元，運費為40元。`
- 金額 1000 元以上：`您的訂單滿1000元，享免運。`

# 注意事項
- 若金額資訊不完整或未提供，先請對方提供訂單金額。
- 若金額含小數，依實際金額判斷門檻，不需四捨五入。
