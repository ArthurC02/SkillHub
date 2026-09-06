---
name: excel-customer-list-dedup-and-phone-mark
description: 清理 Excel 客戶名單時使用：去除重複列，並標記缺少電話的列；當輸入是表格資料且需要保留原始欄位並輸出可直接使用的結果時採用。
---

# 目的
處理使用者提供的 Excel 客戶名單：去除重複列，並標記缺少電話的列。

# 適用時機
當輸入是 Excel 或可轉成表格的客戶名單，且需求明確包含「去重」與「標記缺少電話」時使用。

# 執行原則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# 操作步驟
1. 讀取使用者提供的表格資料。
2. 判定去重依據。
   - 若輸入已明確指定去重規則，直接依該規則處理。
   - 若輸入沒有指定去重規則，將去重規則視為 not given，並在結果中如實反映，不自行補定義。
3. 去除重複列。
   - 只依輸入明示的規則判定重複。
   - 不新增輸入沒有提供的欄位或資料。
4. 標記缺少電話的列。
   - 以輸入中既有的電話欄位為準。
   - 電話值空白、缺值或等同缺失時，標記該列。
   - 標記方式以清楚、可直接檢視的表格欄位或狀態呈現，且不得改變原始資料內容的意義。
5. 保留原始主要欄位資訊。
   - 輸出必須仍是可直接閱讀的表格結果。
   - 只包含輸入提供的欄位與依規則得到的標記資訊。
6. 輸出完成後，交付整理後的結果本身。

# 輸出要求
- 以表格或可直接轉成表格的結構輸出。
- 明確顯示哪些列被去重後保留，哪些列被視為重複。
- 明確顯示哪些列缺少電話。
- 不要只做描述性回覆；要提供整理後的成品。
- 若某項資訊在輸入中未提供，寫 not given。
