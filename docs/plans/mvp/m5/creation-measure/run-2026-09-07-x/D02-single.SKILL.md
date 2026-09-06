---
name: shopify-excel-order-import
description: 'Use when you need to process customer Excel orders into a Shopify backend import flow: convert the spreadsheet to CSV, check for missing fields, and prepare the customer-facing order confirmation. This skill follows the workflow shown in the diagram and is for order intake tasks, not live Shopify actions unless the agent has access.'
---

# Shopify Excel 訂單匯入流程

依照下列流程處理客戶 Excel 訂單：

1. **收到客戶 Excel 訂單**
   - 確認檔案可開啟，且內容是訂單資料。
   - 若檔案無法讀取，先回覆客戶請重新提供可開啟的 Excel 檔。

2. **轉成 CSV 格式**
   - 將 Excel 另存或轉出為 CSV。
   - 保留原始資料欄位與列順序，避免不必要的欄位變動。
   - 若有多工作表，先確認要匯入的是哪一張；通常只轉出實際訂單資料所在的工作表。

3. **檢查缺漏欄位**
   - 檢查匯入 Shopify 所需的必要欄位是否齊全。
   - 至少確認常見訂單欄位是否完整，例如：客戶資訊、商品資訊、數量、收件資料、聯絡方式、地址、備註或其他系統要求欄位。
   - 找出空白、格式錯誤、欄位名稱不一致、重複資料或無法對應的內容。
   - 若發現缺漏，整理成清單，先補齊或回覆客戶補資料，再進入下一步。

4. **匯入 Shopify 後台**
   - 在具備授權與可用介面的情況下，將 CSV 匯入 Shopify 後台。
   - 若沒有登入權限或無法操作後台，明確說明無法直接匯入，並改為提供可匯入的 CSV 與缺漏清單。
   - 匯入前再次確認欄位對應正確，避免錯誤建立訂單。

5. **回覆客戶已入單**
   - 匯入完成後，回覆客戶訂單已建立或已入單。
   - 若有任何缺漏、異常或需要客戶確認的項目，一併說明。

## 執行原則
- 先處理格式，再檢查資料完整性，最後才進行匯入與回覆。
- 不要假設缺漏欄位的內容；缺少資訊時要先向客戶確認。
- 若流程中任何一步無法完成，停在該步並回報原因與下一步需要的資訊。
