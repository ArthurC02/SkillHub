---
name: excel-customer-list-cleanup
description: 清理 Excel 客戶名單中的重複資料，並標記電話缺漏的列；當輸入是一份客戶名單表格且需要去重與缺欄標示時使用。
---

# Excel 客戶名單清理

當使用者提供一份客戶名單表格，並要求去除重複資料、標記缺少電話的列時，依下列規則處理並直接產出成品。

1. 先讀取使用者提供的表格內容。
2. 以輸入中實際出現的欄位為準；只根據欄位內容判斷，不自行新增欄位或資料。
3. 去除重複列：
   - 只依輸入內容判斷重複。
   - 若同一筆資料在表格中重複出現，保留一筆，其餘移除。
4. 標記缺少電話的列：
   - 若電話欄位是空白、缺值或未提供，直接標記該列缺少電話。
   - 標記方式要明確可見，且保留原始資料內容。
5. 產出結果時，直接給出整理後的表格或清單。
6. 不要補電話，不要猜測，不要要求額外資料來源。
7. 只做使用者要求的清理，不增加其他規則。

## 必須遵守

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 輸出要求

- 輸出整理後的結果本身。
- 若電話缺少，直接標示該列缺少電話。
- 若有重複列，移除重複並保留一筆。
- 若某項資訊未提供，寫 `not given`。