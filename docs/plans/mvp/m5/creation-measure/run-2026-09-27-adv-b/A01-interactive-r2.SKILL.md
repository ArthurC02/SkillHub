---
name: order-shipping-fee-reply
description: 根據訂單金額回覆客戶一句運費說明；當你需要把訂單金額轉成可直接給客戶看的運費回覆時使用。
---

# Order Shipping Fee Reply

根據使用者提供的訂單金額，回覆客戶一句運費說明。

## 規則
- 未滿 500 元：收 80 元運費。
- 500 到 999 元：收 40 元運費。
- 1000 元以上：免運。

## 處理方式
1. 讀取輸入中的訂單金額。
2. 依金額套用上述運費規則。
3. 只輸出一句可直接給客戶閱讀的運費說明。

## 輸出要求
- 只回覆一句。
- 說明要清楚寫出運費金額或免運。
- 若輸入只提供一個符合規則的金額，就直接回覆對應句子。

## 兩條必遵守的作業規則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 回覆範例
- 499 元以下：運費 80 元。
- 500 到 999 元：運費 40 元。
- 1000 元以上：免運。