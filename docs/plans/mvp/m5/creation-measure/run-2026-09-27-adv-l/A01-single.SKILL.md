---
name: shipping-fee-reply
description: Use when you need to reply to a customer with the shipping fee based on order amount tiers and provide a one-sentence explanation.
---

# 目的
根據訂單金額回覆客戶運費，並用一句話說明規則。

# 判斷規則
1. 先取得訂單金額。
2. 依金額套用以下運費：
   - 未滿 500 元：運費 80 元
   - 500 元到 999 元：運費 40 元
   - 1000 元以上：免運
3. 回覆時只用一句完整說明，清楚寫出金額區間與對應運費。

# 回覆格式
- 金額未滿 500 元：`訂單未滿500元，運費80元。`
- 金額 500 到 999 元：`訂單滿500元未滿1000元，運費40元。`
- 金額 1000 元以上：`訂單滿1000元以上，免運費。`

# 注意事項
- 不要輸出多句說明。
- 不要加入額外優惠、例外條件或未提供的資訊。
- 若訂單金額不明，先請對方提供訂單金額，再依規則回覆。
