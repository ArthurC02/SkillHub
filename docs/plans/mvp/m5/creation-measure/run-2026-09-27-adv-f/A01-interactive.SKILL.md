---
name: order-shipping-reply
description: 根據訂單金額回覆客戶一句運費說明；當你需要快速產生可直接對外使用的運費回覆時使用。
---

# Order Shipping Reply

根據使用者提供的訂單金額，直接產生一句可回覆客戶的運費說明。

## 執行方式

1. 讀取輸入中的訂單金額。
2. 依金額判斷運費規則：
   - 未滿 500 元：運費 80 元。
   - 500 到 999 元：運費 40 元。
   - 1000 元以上：免運。
3. 只輸出一句話，內容要能直接發給客戶。
4. 若輸入中有多個金額，分別依各自金額套用同一規則，仍然各自回覆成一句話。
5. 句子可自然表達，但必須清楚包含對應的費用結果。

## 輸出要求

- 只輸出最終回覆文字，不要列點、表格或多行。
- 回覆要簡潔，適合直接貼給客戶。

## 必守規則

- never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact.
- when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 檢核重點

- 金額小於 500 時，回覆必須對應 80 元運費。
- 金額介於 500 到 999 時，回覆必須對應 40 元運費。
- 金額大於等於 1000 時，回覆必須對應免運。
- 回覆必須只有一句話。