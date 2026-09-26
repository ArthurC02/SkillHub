---
name: order-shipping-fee-reply
description: 依新臺幣訂單金額套用固定運費級距，產生一句繁體中文客服回覆。當使用者提供訂單金額並要求說明運費時使用。
---

# 訂單運費客服回覆

根據輸入中的新臺幣訂單金額，直接產生一句可傳送給客戶的繁體中文運費說明。

## 執行規則

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- 從輸入讀取明確提供的訂單金額，不計入輸入未提供的折扣、商品例外、離島加價或其他物流規則。
- 若訂單金額未滿 500 元，回覆該筆訂單的運費為 80 元。
- 若訂單金額為 500 至 999 元，回覆該筆訂單的運費為 40 元。
- 若訂單金額為 1000 元以上，回覆該筆訂單免運。
- 只說明該筆訂單適用的結果，不列出或提及其他運費級距。
- 使用清楚、禮貌、自然的繁體中文，並將完整回覆限制為一句話。
- 不加入折扣、離島、商品限制、配送方式或任何輸入未提供的資訊。
- 直接輸出完成的客服回覆，不加標題、前言、分析、計算過程、項目符號或引號。
- 若輸入完全沒有訂單金額，只提出一個簡短且可回答的問題，請使用者提供以新臺幣計算的訂單金額；不要自行猜測。

## 回覆形式

依適用級距採用以下單句形式，並只輸出其中一句：

- 未滿 500 元：`您好，您的訂單運費為 80 元。`
- 500 至 999 元：`您好，您的訂單運費為 40 元。`
- 1000 元以上：`您好，您的訂單符合免運條件。`