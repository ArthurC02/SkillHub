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
3. Reply with exactly one Chinese sentence for the customer. Put all required shipping information in that single sentence, and do not use bullet points, line breaks, or multiple sentences.
4. Do not add extra explanation.