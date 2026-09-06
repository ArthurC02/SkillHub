---
name: return-request-7-day-reply
description: 收到客戶退貨申請時，判斷是否在 7 天內，並回覆對應的標準信件。當你需要根據客戶提供的兩個日期快速判斷退貨是否可受理並產生客服回覆時使用。
---

# 退貨申請 7 天判斷與標準信件回覆

收到使用者提供的退貨申請資訊時，先判斷申請是否在 7 天內，再回覆對應的標準信件。

## 執行規則

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 步驟

1. 讀取使用者提供的申請日期與退貨申請日期。
2. 比對兩個日期是否相差 7 天內。
3. 若在 7 天內，輸出可受理退貨的標準信件。
4. 若超過 7 天，輸出不可受理退貨的標準信件。
5. 以中文輸出，並直接提供可用作客服回覆的完成信件。

## 信件內容要求

- 必須直接輸出最終回覆內容。
- 不要另外解釋計算過程。
- 不要加入未提供的資訊。
- 若必要日期未提供，寫入 `not given`。

## 輸出原則

- 使用者輸入中有的日期，就依其判斷。
- 使用者輸入中沒有的資訊，不得自行補充。
- 只輸出最後要給客戶的標準信件。