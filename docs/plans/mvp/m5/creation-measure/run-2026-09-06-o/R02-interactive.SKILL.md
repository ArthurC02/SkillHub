---
name: excel-customer-dedupe-phone-tagging
description: Use this skill when you need to clean an Excel customer list by removing duplicate rows and marking rows with missing phone numbers. It produces a direct cleaned result from the list you provide.
---

# Excel 客戶名單去重與缺電話標記

處理一份客戶名單時，直接輸出成品結果：去除重複列，並標記缺少電話的列。

## 你要做的事
1. 讀取使用者提供的名單內容。
2. 去除重複列。
3. 找出缺少電話的列並加上標記。
4. 直接輸出整理後的結果。

## 執行規則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- 對於「重複」與「缺少電話」的判準，只依使用者輸入中明確給出的內容；若未給出，寫 `not given`。
- 不要自行補電話號碼、姓名、公司名或其他缺失資料。
- 不要詢問補充資訊；若輸入本身不足，只能在結果中寫明 `not given`。

## 輸出要求
- 以清楚的表格或條列呈現整理後結果。
- 每筆保留必要欄位，並清楚標出是否缺少電話。
- 若有重複列，輸出去重後的結果。
- 若輸入已經是成品格式，就直接按成品格式整理，不要改寫成說明文字。

## 處理方式
- 先找出名單中的每一列。
- 再比較哪些列重複。
- 然後檢查電話欄位是否空白或未提供。
- 最後輸出去重與標記後的完整結果。

## 缺少或未定義資訊
- 重複判準：not given
- 缺少電話判準：not given
- 輸出檔案格式：not given

## 交付原則
- 只輸出整理後的成品。
- 不要附加額外說明、推理過程或工作筆記。