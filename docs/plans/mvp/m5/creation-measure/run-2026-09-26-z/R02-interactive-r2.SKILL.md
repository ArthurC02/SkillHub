---
name: excel-customer-list-deduper
description: 去除 Excel 客戶名單中的重複列，並標記電話欄缺失的資料列；當你要整理客戶名單並保留原始其他欄位時使用。
---

# Excel 客戶名單去重與缺少電話標記

## 你要做的事
處理使用者提供的 Excel 客戶名單，完成兩件事：
1. 去除重複資料
2. 標記缺少電話的列

## 必須遵守
- 只使用輸入中實際提供的欄位與資料。
- 不要假設欄名固定；先辨識哪些欄位存在，再決定如何處理。
- 去重規則以使用者提供的資料為準；如果輸入沒有明確指定去重依據，就以整列內容完全相同作為重複判定。
- 電話欄位若不存在，輸出要明確寫出 `not given`，不要自行補欄名。
- 只輸出完成後的整理結果；不要輸出規則解釋、計畫或請求額外權限。

## 處理步驟
1. 讀取輸入的 Excel 內容。
2. 找出可用來判斷重複的欄位；若沒有額外指定，就以整列內容比對。
3. 移除完全重複的列，保留第一筆出現的列。
4. 找出電話欄；若電話欄存在，檢查哪些列的電話是空白。
5. 對電話空白的列加上明確標記，例如新增一欄 `缺少電話`，值填入 `是`；若輸入已指定其他標記方式，就依輸入方式處理。
6. 保留其他欄位內容不變。
7. 輸出整理後的結果，格式與輸入一致；若格式無法判定，寫成可回填的表格內容。

## 輸出要求
- 直接給出整理後的資料，且第一行先寫一句摘要，明確列出「已去重」與「已標記缺少電話」，讓處理結果可一眼確認。
- 對已去重與已標記的列保持清楚可辨。
- 若某個必要資訊不存在，就寫 `not given`。

## 兩條硬性規則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 備註
若輸入同時包含重複列與電話空白列，先完成去重，再標記缺少電話。