---
name: shopify-excel-order-import
description: Use this skill when you receive customer orders in Excel and need to convert them into CSV, check for missing fields, import them into the Shopify admin, and then confirm the order has been entered.
---

# Excel 訂單匯入 Shopify 流程

依照下列流程處理客戶訂單：

1. **收到客戶 Excel 訂單**
   - 先確認檔案可開啟，且內容是訂單資料。
   - 若檔案格式不是 Excel，先請對方提供可匯出的表格檔。

2. **轉成 CSV 格式**
   - 將 Excel 另存或匯出為 CSV。
   - 匯出前保留原始 Excel 檔，避免後續需要回溯。
   - 確認 CSV 編碼與分隔符號可被後續系統正確讀取。

3. **檢查缺漏欄位**
   - 檢查訂單匯入所需欄位是否完整。
   - 特別留意常見缺漏：客戶姓名、Email、商品名稱或 SKU、數量、收件資訊、地址、電話、備註。
   - 若有缺漏或格式不一致，先整理修正；無法判定時，回頭向客戶確認。

4. **匯入 Shopify 後台**
   - 使用 Shopify 後台的訂單匯入流程，將整理好的 CSV 上傳。
   - 匯入後檢查系統回饋，確認沒有欄位對應錯誤或資料列失敗。
   - 若有失敗列，根據錯誤訊息修正 CSV 後重新匯入。

5. **回覆客戶已入單**
   - 確認訂單已成功建立後，再回覆客戶。
   - 回覆內容應包含已完成入單的確認；若有異常，則說明缺漏或需要補件的項目。

## 執行原則
- 先整理資料，再匯入；不要直接把未檢查的 Excel 送入後台。
- 若流程中任何一步無法完成，先停下來補齊資料或確認格式。
- 保留原始檔與整理後檔案，方便追蹤與修正。
