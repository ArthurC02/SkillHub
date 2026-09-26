---
name: shipping-fee-reply
description: Use when you need to reply to a customer with the shipping fee based on order amount. It determines the correct fee tier and produces a single-sentence explanation in Chinese.
---

# 目的
根據訂單金額回覆客戶運費，並以一句話說明。

# 判斷規則
- 訂單金額 **未滿 500 元**：運費 **80 元**
- 訂單金額 **500 到 999 元**：運費 **40 元**
- 訂單金額 **1000 元以上**：**免運**

# 執行步驟
1. 先確認訂單金額。
2. 依金額套用對應運費規則。
3. 用一句完整、禮貌的中文回覆客戶。
4. 回覆內容只需包含運費結果與簡短說明，不要加入多餘資訊。

# 回覆格式
- 未滿 500 元：`您好，您的訂單未滿 500 元，運費為 80 元。`
- 500 到 999 元：`您好，您的訂單滿 500 元未滿 1000 元，運費為 40 元。`
- 1000 元以上：`您好，您的訂單滿 1000 元，享免運優惠。`

# 注意事項
- 金額邊界要正確判斷：500 元屬於 40 元運費，1000 元屬於免運。
- 若只收到金額，直接依規則回覆，不要額外詢問。
