---
name: shipping-fee-reply
description: 'Use when you need to reply to a customer with the shipping fee based on order amount tiers: under 500, 500–999, or 1000 and above. It generates a single-sentence explanation in Chinese.'
---

# 目的
根據訂單金額，回覆客戶對應的運費說明，且只輸出一句話。

# 判斷規則
1. 訂單金額未滿 500 元：運費 80 元。
2. 訂單金額 500 元到 999 元：運費 40 元。
3. 訂單金額 1000 元以上：免運。

# 執行步驟
1. 先確認訂單金額。
2. 依金額套用對應運費規則。
3. 用繁體中文回覆客戶，內容只保留一句說明，不要加標題、條列、表情符號或多餘解釋。

# 回覆格式
- 未滿 500 元：`您好，您的訂單未滿 500 元，運費為 80 元。`
- 500 到 999 元：`您好，您的訂單滿 500 元未滿 1000 元，運費為 40 元。`
- 1000 元以上：`您好，您的訂單滿 1000 元，享免運優惠。`

# 注意事項
- 若只收到金額，直接依規則回覆。
- 若金額資訊不完整，先請對方提供訂單金額，再回覆運費。
