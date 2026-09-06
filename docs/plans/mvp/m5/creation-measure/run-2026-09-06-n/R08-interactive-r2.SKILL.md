---
name: return-request-7-day-reply
description: When a customer return request includes dates or enough timing information, determine whether it is within 7 days and draft the corresponding standard Chinese reply. Use this skill when you need a ready-to-send return response based only on the request text.
---

# 退貨申請 7 天內判斷與標準回覆

收到客戶退貨申請時，直接根據輸入文字判斷是否在 7 天內，並輸出對應的中文標準回覆信。

## 執行原則

- 只根據輸入文字判斷。
- 不補寫未提供的政策細節。
- 不自動推定其他退貨條件。
- 若輸入缺少足以判斷是否在 7 天內的資訊，回覆「not given」，並指出缺少哪些資訊。

## 你要做的事

1. 讀取使用者提供的退貨申請文字。
2. 找出申請日期與收到/購買日期，或其他足以判斷是否在 7 天內的資訊。
3. 判斷是否在 7 天內。
4. 輸出一封可直接使用的中文標準客服回覆信。

## 回覆規則

- 如果在 7 天內：輸出可受理版本。
- 如果超過 7 天：輸出不可受理版本，並明確說明已超過 7 天期限。
- 回覆內容要清楚寫出判斷結果。
- 回覆內容不要加入與退貨無關的政策條款。
- 回覆內容要能直接作為客服回覆使用。

## 缺少資訊時

- 若輸入沒有提供足以判斷的日期或時間資訊，明確寫出「not given」。
- 只指出缺少的資訊，不要自行推測。

## 內建作業要求

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 輸出形式

- 直接輸出客服回覆信正文。
- 不要額外附上分析過程。
- 不要輸出規則摘要。
