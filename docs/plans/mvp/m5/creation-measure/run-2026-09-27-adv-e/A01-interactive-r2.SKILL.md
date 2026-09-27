---
name: order-shipping-reply
description: 根據訂單金額回覆客戶運費說明；當需要快速產生一句中文運費回覆時使用。
---

# Order Shipping Reply

Use this skill when you need to reply to a customer with the shipping fee based on an order amount.

## Instructions

1. Read the order amount from the input.
2. Determine the shipping fee:
   - If the amount is less than 500, the shipping fee is 80.
   - If the amount is from 500 to 999, the shipping fee is 40.
   - If the amount is 1000 or more, shipping is free.
3. Reply in one Chinese sentence that clearly states the shipping fee or that shipping is free.
4. Do not add extra explanation.
