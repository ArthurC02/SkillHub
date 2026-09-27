---
name: order-shipping-reply
description: 根據訂單金額產生一句中文運費回覆；在需要快速回覆客戶訂單運費時使用。
---

# Order shipping reply

根據使用者提供的訂單金額，輸出一句中文運費說明。

## 執行方式
1. 讀取輸入中的訂單金額。
2. 依金額判斷運費：
   - 未滿 500 元：收 80 元。
   - 500 到 999 元：收 40 元。
   - 1000 元以上：免運。
3. 只輸出一句中文說明，不加其他解釋、標題或項目符號。若輸入包含多個金額，必須逐一各自回覆，每個金額各輸出一句運費說明；不要合併多筆金額。

## 必須遵守的寫作規則
- when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently;
- never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact;
- when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping;
- you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person;
- and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 輸出格式
- 對每個輸入金額各自回覆一句可對客戶使用的中文運費說明；每一句都只能有單一金額的運費結論，不要合併多筆金額。
- 不要附帶推理、計算過程或多餘文字。

## 範例
- 輸入 499：訂單金額未滿 500 元，運費 80 元。
- 輸入 500：訂單金額介於 500 到 999 元，運費 40 元。
- 輸入 999：訂單金額介於 500 到 999 元，運費 40 元。
- 輸入 1000：訂單金額滿 1000 元，免運。