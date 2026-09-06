---
name: excel-customer-dedup-phone-flagger
description: Clean Excel customer lists by removing duplicates and flagging rows with missing phone numbers. Use this when you need a direct spreadsheet-processing workflow for a customer list.
---

# Excel 客戶名單去重與缺電話標記

你會收到一份 Excel 客戶名單。你的工作是直接產出處理後的結果：去除重複列，並標記電話缺少的列。

## 必做事項
1. 讀取使用者提供的 Excel 名單。
2. 找出重複資料並去重。
3. 找出電話欄缺少的列並標記。
4. 保留其他原始欄位內容，除非輸入本身沒有提供。
5. 直接輸出完成後的結果。

## 判定規則
- 重複的判定依據：用輸入中明確提供的規則；如果輸入沒有說明，就寫 `not given`。
- 缺少電話：電話欄空白、只有空白字元，或欄位不存在時，視為缺少電話。
- 若輸入沒有說明要保留哪一筆重複資料，就保留第一筆出現的列。

## 輸出要求
- 輸出應清楚呈現去重後的名單。
- 對缺少電話的列加上明確標記，例如在新欄位寫 `電話缺少`。
- 若有欄位內容在輸入中沒有提供，寫 `not given`。
- 直接交付成品，不要解釋操作步驟。

## 工作方式
1. 先確認輸入是否包含可辨識的客戶資料與電話欄。
2. 依輸入提供的重複判定規則去重。
3. 檢查每列電話是否缺少，並標記。
4. 輸出整理後的完整結果。

## 給最終輸出的兩條指令
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
