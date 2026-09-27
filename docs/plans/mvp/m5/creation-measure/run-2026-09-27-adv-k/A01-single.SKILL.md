---
name: order-shipping-fee-reply
description: When asked to reply to a customer about shipping fees based on order amount, generate a one-sentence explanation using the stated thresholds.
---

# 目的
根據訂單金額，回覆客戶對應的運費說明，且只輸出一句完整說明。

# 規則
- 訂單金額未滿 500 元：運費 80 元。
- 訂單金額 500 到 999 元：運費 40 元。
- 訂單金額 1000 元以上：免運。

# 執行步驟
1. 讀取使用者提供的訂單金額。
2. 判斷金額落在哪一個區間。
3. 依對應規則組成一句中文回覆。
4. 回覆內容需簡潔、禮貌，且只輸出一句話，不要加標題、條列或額外說明。

# 回覆格式
- 未滿 500 元：`您好，訂單未滿 500 元，運費為 80 元。`
- 500 到 999 元：`您好，訂單滿 500 元未滿 1000 元，運費為 40 元。`
- 1000 元以上：`您好，訂單滿 1000 元即可免運。`

# 注意事項
- 若金額剛好是 500 元，歸入 500 到 999 元區間。
- 若金額剛好是 1000 元，歸入免運區間。
- 不要自行更改門檻或金額。
