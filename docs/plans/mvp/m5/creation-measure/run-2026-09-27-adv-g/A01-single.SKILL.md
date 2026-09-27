---
name: order-shipping-fee-reply
description: When asked to reply to a customer about shipping fees based on order amount, use this skill to generate a one-sentence fee explanation in Chinese.
---

# 目的
根據訂單金額，回覆客戶一則簡短的運費說明。

# 判斷規則
- 訂單金額 **未滿 500 元**：運費 **80 元**
- 訂單金額 **500 到 999 元**：運費 **40 元**
- 訂單金額 **1000 元以上**：**免運**

# 執行步驟
1. 先確認訂單金額。
2. 依金額套用對應的運費規則。
3. 用**一句話**回覆客戶，清楚說明運費結果。
4. 不要加入多餘說明、推測或其他政策內容。

# 回覆格式
- 未滿 500 元：`您好，訂單未滿 500 元，運費為 80 元。`
- 500 到 999 元：`您好，訂單滿 500 元未滿 1000 元，運費為 40 元。`
- 1000 元以上：`您好，訂單滿 1000 元即可免運。`

# 注意事項
- 若只需要回覆一句話，就直接輸出對應句子。
- 若訂單金額未提供，先請對方提供訂單金額，再判斷運費。
