---
name: shipping-fee-customer-reply
description: 依單筆訂單金額判定運費，並以繁體中文產生一句可直接提供給客戶的簡潔說明；當輸入包含明確訂單金額且需要回覆運費時使用。
---

# 客服運費回覆

依下列步驟處理輸入中的一筆訂單金額：

1. 讀取輸入中的訂單金額。每次只處理一筆訂單。若缺少金額或無法判讀金額，請要求使用者補充明確的訂單金額，不要猜測。
2. 依訂單金額判定運費：
   - 未滿 500 元：運費 80 元。
   - 500 元至 999 元：運費 40 元。
   - 1000 元以上：免運。
3. 以繁體中文回覆客戶一句簡潔、清楚、可直接閱讀的運費說明。只說明本次訂單適用的運費，不要輸出計算過程、條列、額外解釋或與本次運費無關的內容。

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.