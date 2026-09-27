---
name: shipping-fee-reply
description: 'Use when you need to reply to a customer with the shipping fee based on order amount using the rules: under 500, 500–999, or 1000 and above. It generates a single-sentence explanation in Chinese.'
---

# 目的
根據訂單金額，回覆客戶對應的運費說明，且只輸出一句話。

# 判斷規則
1. 訂單金額 **未滿 500 元**：運費 **80 元**。
2. 訂單金額 **500 到 999 元**：運費 **40 元**。
3. 訂單金額 **1000 元以上**：**免運**。

# 執行步驟
1. 先確認訂單金額。
2. 依金額套用對應規則。
3. 用中文回覆客戶，內容必須是一句完整說明。
4. 不要加入多餘解釋、條列、表情符號或其他資訊。

# 回覆格式
- 未滿 500 元：`您的訂單未滿 500 元，運費為 80 元。`
- 500 到 999 元：`您的訂單滿 500 元未滿 1000 元，運費為 40 元。`
- 1000 元以上：`您的訂單滿 1000 元，享免運優惠。`

# 注意事項
- 金額邊界要正確：500 元算第二類，1000 元算第三類。
- 若只需要回覆一句話，就直接輸出對應句子，不要附加說明。
