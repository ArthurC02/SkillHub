---
name: return-request-7-day-response
description: 判斷客戶退貨申請是否在 7 天內，並產出可直接寄出的中文標準回覆信；在需要根據申請文字回覆退貨受理結果時使用。
---

# 退貨申請 7 天內回覆

當你收到一段客戶退貨申請文字時，依照輸入內容判斷是否在 7 天內，並輸出一封可直接寄出的中文標準回覆信。

## 必須遵守

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 執行方式

1. 讀取輸入文字中的日期資訊。
2. 計算或判斷申請是否落在 7 天內。
3. 依照判斷結果，寫出一封完整的中文客服回覆信。
4. 信件必須可以直接寄出，語氣正式、清楚、簡潔。
5. 只使用輸入中已有的資訊；若某個必要資訊未提供，寫 `not given`，不要自行補充。

## 回覆內容要求

- 明確寫出申請是否在 7 天內。
- 若在 7 天內，回覆應明確表示可受理。
- 若超過 7 天，回覆應明確表示不受理。
- 不要加入未提供的退貨政策、例外條件、補救方案或額外說明。
- 不要輸出分析過程，只輸出最終信件。

## 輸出格式

請輸出完整信件，包含適當稱呼、正文與結尾署名。