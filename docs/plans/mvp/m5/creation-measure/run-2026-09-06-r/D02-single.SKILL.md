---
name: shopify-excel-order-import
description: Use this skill when you receive customer orders in Excel and need to convert them into CSV, check for missing fields, import them into the Shopify admin, and then reply to the customer that the order has been entered.
---

# Excel 訂單匯入 Shopify 流程

依照下列流程處理客戶提供的 Excel 訂單：

1. **收到客戶 Excel 訂單**
   - 確認檔案可開啟，且內容是訂單資料。
   - 若檔案無法讀取、格式錯誤或不是 Excel，先回覆客戶請重新提供可用檔案。

2. **轉成 CSV 格式**
   - 將 Excel 另存或匯出為 CSV。
   - 保留欄位名稱與資料內容，避免在轉檔時改變欄位順序或資料格式。
   - 若有多個工作表，只處理實際包含訂單資料的工作表。

3. **檢查缺漏欄位**
   - 檢查 CSV 是否包含匯入 Shopify 所需的必要欄位。
   - 找出空白、缺值、格式不正確或欄位名稱不一致的地方。
   - 若有缺漏，先整理出問題清單，必要時請客戶補資料後再繼續。
   - 若資料完整，進入下一步。

4. **匯入 Shopify 後台**
   - 使用 Shopify 後台的訂單匯入流程，將 CSV 上傳並完成匯入。
   - 匯入前再次確認欄位對應正確，避免資料寫入錯誤。
   - 若匯入失敗，根據錯誤訊息修正 CSV 後重試；若無法自行修正，整理錯誤原因回報。

5. **回覆客戶已入單**
   - 匯入成功後，通知客戶訂單已完成入單。
   - 回覆內容應簡潔明確，必要時附上已完成處理的說明。

## 處理原則
- 先檢查資料完整性，再進行匯入。
- 不要在未確認欄位與格式前直接匯入。
- 若流程中任何一步無法完成，先停下來整理問題，不要假設缺漏資料。
