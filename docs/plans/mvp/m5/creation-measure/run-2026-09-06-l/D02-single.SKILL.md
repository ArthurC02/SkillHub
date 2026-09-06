---
name: shopify-order-import-from-excel
description: Use when you receive customer orders in Excel and need to convert them into CSV, check for missing fields, import them into the Shopify admin, and then confirm the order has been entered.
---

# Excel 訂單匯入 Shopify 流程

依照圖中的流程執行：收到客戶 Excel 訂單 → 轉成 CSV 格式 → 檢查缺漏欄位 → 匯入 Shopify 後台 → 回覆客戶已入單。

## 執行步驟

1. **接收並確認 Excel 訂單**
   - 先確認收到的是客戶提供的 Excel 訂單檔。
   - 檢查檔案是否可開啟、是否包含訂單資料、欄位名稱是否清楚。

2. **轉成 CSV 格式**
   - 將 Excel 另存或匯出為 CSV。
   - 優先使用 Shopify 匯入所需的 CSV 格式。
   - 保留原始 Excel 檔作為備份，不直接覆蓋。

3. **檢查缺漏欄位**
   - 檢查 CSV 是否有必要欄位缺失、空白或格式錯誤。
   - 常見需確認的內容包括：客戶姓名、聯絡方式、商品名稱、數量、地址、郵遞區號、備註等。
   - 若發現缺漏：
     - 先整理出缺少的欄位清單。
     - 若可從原始資料補齊，先補齊再繼續。
     - 若無法補齊，先回覆客戶或內部確認後再匯入。

4. **匯入 Shopify 後台**
   - 使用 Shopify 後台的匯入功能上傳 CSV。
   - 匯入前再次確認欄位對應正確，避免資料錯置。
   - 匯入後檢查系統回饋，確認是否成功建立訂單或是否有錯誤訊息。
   - 若匯入失敗，根據錯誤訊息修正 CSV 後重新匯入。

5. **回覆客戶已入單**
   - 確認訂單已成功進入 Shopify 後，回覆客戶已完成入單。
   - 回覆內容應簡潔明確，必要時附上訂單編號或處理狀態。

## 注意事項

- 若 Shopify 後台需要登入，而目前沒有可用的登入權限，應先請求可用帳號或由有權限的人員操作。
- 若 CSV 格式要求不明，先查明 Shopify 匯入規格再進行。
- 若圖片中的流程與實際欄位規則不一致，以實際系統要求為準。
