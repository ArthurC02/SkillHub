---
name: return-request-standard-reply
description: 在收到客戶退貨申請、且需要判斷是否在 7 天內時，產生可直接寄出的中文標準回覆信件。用在只有申請內容與日期資訊可供判斷的情況，不自行補資料。
---

# 目的

當你收到客戶退貨申請時，判斷是否在 7 天內，並回覆對應的標準信件。

## 你要做的事

1. 讀取輸入中的退貨申請內容。
2. 找出可用來判斷的日期資訊。
3. 判斷該申請是否在 7 天內。
4. 直接輸出對應的標準回覆信件。

## 執行規則

- 只根據輸入中的日期資訊判斷。
- 不自行補資料。
- 不查詢外部來源。
- 若輸入沒有足夠日期資訊，回覆時寫出缺少的資訊為「not given」。
- 只輸出可直接寄出的成品，不要輸出分析、步驟或說明。

## 兩種標準回覆

### 1. 7 天內

如果判斷結果是在 7 天內，輸出一封中文標準回覆信件，內容明確表示申請符合 7 天內退貨條件，並按輸入中的商品或申請資訊自然帶入已有內容。

### 2. 超過 7 天

如果判斷結果是超過 7 天，輸出一封中文標準回覆信件，內容明確表示申請已超過 7 天，並說明無法依 7 天內退貨規則處理。

## 寫作要求

- 信件要可直接寄出。
- 保持中文。
- 依輸入中的資訊決定內容，不加上未提供的事實。
- 若某個必要欄位在輸入中沒有提供，就寫「not given」。

## 必須遵守的兩條規則

use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.