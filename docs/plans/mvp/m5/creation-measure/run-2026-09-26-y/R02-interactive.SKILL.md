---
name: excel-customer-list-cleanup
description: 去除 Excel 客戶名單中的重複資料，並標記缺少電話的列；當你收到客戶名單整理需求時使用。
---

# 目標
處理使用者提供的 Excel 客戶名單內容：去除重複資料，並標記缺少電話的列。

# 執行方式
1. 讀取使用者在輸入中提供的客戶名單資料。
2. 依資料本身可見的內容判斷重複列，將重複資料去除。
3. 檢查每一列的電話欄：
   - 電話欄有內容：不標記為缺少電話。
   - 電話欄空白：加上可辨識的缺少電話標記。
4. 保留原始資料可辨識的欄位結構，讓輸出仍能對應回原始資料列。
5. 直接輸出整理後的結果，不要改寫成說明文字。

# 輸出要求
- 產出一份整理後的資料。
- 標記方式要清楚，讓人可辨識哪些列缺少電話。
- 若使用者提供的是表格、CSV 或貼上的 Excel 內容，就以同樣的資料型態整理後輸出。

# 必須遵守
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# 限制
- 只依據輸入中可見的資料處理，不要自行補欄位、補資料或猜測缺漏內容。
- 若輸入未提供可處理的客戶名單內容，回覆缺少必要輸入，並指出缺少的是資料本身。