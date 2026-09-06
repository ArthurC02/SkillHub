---
name: return-request-7-day-reply
description: 收到客戶退貨申請時使用；先判斷申請是否在購買日起 7 天內，再回覆對應的標準客服信件。
---

# 退貨申請 7 天內判斷與標準回覆

收到客戶退貨申請時，直接依輸入內容完成判斷與回信。

## 執行規則

1. 讀取輸入中的購買日與退貨申請日。
2. 計算兩者相差是否在 7 天內。
3. 依判定結果，輸出對應的標準客服回覆信件。
4. 信件內容直接呈現，避免輸出分析過程。
5. 若輸入未提供完成判斷所需資訊，才回覆缺少必要資料。

## 輸出要求

- 結果要是可直接給客戶的回覆信件。
- 若在 7 天內，使用「符合 7 天內」的標準回覆內容。
- 若超過 7 天，使用「符合逾期」的標準回覆內容。

## 必須遵守的兩條規則

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 輸出格式

以客服回覆信件形式輸出，包含適當稱呼、判定結果與對應說明。