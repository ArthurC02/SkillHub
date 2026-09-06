---
name: excel-customer-list-deduper-phone-marker
description: 處理 Excel 客戶名單時，去除重複列並標記缺少電話的列；當你要把一份客戶名單整理成去重後、且能明確看出缺漏電話的結果時使用。
---

# Excel 客戶名單去重與缺少電話標記

當你收到一份 Excel 客戶名單，需要去除重複列並標記缺少電話的列時，直接依下列步驟完成整理，並輸出整理後的結果。

## 工作原則

- 只使用輸入中提供的資料。
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- 不要自行補欄位、不要猜測電話號碼、不要推斷輸入沒有明寫的清理規則。

## 處理步驟

1. 讀取輸入中的客戶名單資料。
2. 找出重複列，並依輸入資料可直接判定的方式去除重複。
3. 找出電話欄位缺少值的列，並為這些列加上明確標記。
4. 輸出整理後的名單，讓重複列已被去除、缺少電話的列已被標記。

## 輸出要求

- 必須呈現整理後的結果本身。
- 必須清楚看出哪些列被去重，哪些列缺少電話。
- 如果電話欄位是空白，將其視為缺少電話並標記。
- 如果輸入中沒有某些資訊，寫 'not given'。

## 注意事項

- 不要加入輸入沒有提供的額外清理步驟。
- 不要將未提供的資料補齊。
- 不要回覆分析過程或操作說明；只輸出整理後的成品。