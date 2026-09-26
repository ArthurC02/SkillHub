---
name: shopify-order-import-from-excel
description: 'Use when you need to process customer orders from an Excel file into Shopify: convert the spreadsheet to CSV, check for missing required fields, import the data into Shopify admin, and then confirm the customer that the order has been entered.'
---

# Shopify 訂單匯入流程

依照圖中的流程處理客戶 Excel 訂單：先轉成 CSV，再檢查缺漏欄位，接著匯入 Shopify 後台，最後回覆客戶已入單。

## 流程步驟

1. **收到客戶 Excel 訂單**
   - 先確認檔案可開啟，且內容是訂單資料。
   - 若檔案無法讀取、格式損壞或不是 Excel，先請客戶重新提供可用檔案。

2. **轉成 CSV 格式**
   - 將 Excel 另存或匯出為 CSV。
   - 匯出前確認工作表是正確的訂單表。
   - 保留必要欄位名稱與資料列，不要在轉檔時改動欄位意義。

3. **檢查缺漏欄位**
   - 檢查每筆訂單是否缺少匯入 Shopify 所需欄位。
   - 常見需檢查的內容包括：客戶資訊、商品資訊、數量、地址、聯絡方式、付款或配送相關欄位。
   - 若有缺漏或格式不正確：
     - 先整理出缺少的欄位與受影響的列。
     - 回覆客戶補齊資料，或先修正後再繼續。
   - 若資料完整，才進入下一步。

4. **匯入 Shopify 後台**
   - 使用 Shopify 後台的對應匯入功能，將 CSV 上傳並建立訂單。
   - 匯入前再次確認欄位對應正確，避免資料對錯欄。
   - 匯入後檢查結果是否成功，並確認沒有錯誤列或失敗項目。
   - 若匯入失敗，根據錯誤訊息回到 CSV 檢查與修正。

5. **回覆客戶已入單**
   - 在確認匯入成功後，通知客戶訂單已建立或已入單。
   - 若有未完成項目，也要清楚告知目前狀態與需要客戶補充的資訊。

## 處理原則

- 任何缺漏欄位都先處理，不要直接匯入。
- 匯入前後都要保留可追蹤的檔案版本。
- 若 Shopify 後台匯入功能不可用，則只能先完成前置整理與檢查，並回報無法完成匯入的原因。
