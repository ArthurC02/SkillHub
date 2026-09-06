---
name: shopify-excel-order-import
description: Use this skill when you receive customer orders in Excel and need to convert them into a CSV, check for missing fields, import them into the Shopify admin, and then reply that the order has been entered.
---

# Excel 訂單匯入 Shopify 流程

依照下列流程處理收到的客戶 Excel 訂單：

1. **收到客戶 Excel 訂單**
   - 確認檔案可開啟，且內容是訂單資料。
   - 若檔案無法讀取、格式異常，或不是 Excel 訂單，先回覆需要可用的訂單檔案。

2. **轉成 CSV 格式**
   - 將 Excel 內容轉存為 CSV。
   - 保留欄位名稱與資料順序，避免在轉檔時遺失資訊。
   - 若有多工作表，先確認要匯入的是哪一張。

3. **檢查缺漏欄位**
   - 檢查匯入 Shopify 所需的必要欄位是否完整。
   - 特別留意常見缺漏：顧客姓名、Email、商品名稱、數量、規格、地址、電話、運送方式、價格或 SKU。
   - 若發現缺漏或格式不一致，先整理出問題清單，請求補齊後再匯入。
   - 若欄位名稱與 Shopify 匯入格式不一致，先對應並標準化欄位。

4. **匯入 Shopify 後台**
   - 使用已整理好的 CSV 進行 Shopify 後台匯入。
   - 匯入前再次確認欄位對應、編碼與分隔符號正確。
   - 若匯入失敗，根據錯誤訊息回頭檢查 CSV 格式或缺漏欄位。
   - 若此環境無法直接登入或操作 Shopify 後台，明確說明無法代為登入，並提供可執行的匯入檢查清單與 CSV 整理結果。

5. **回覆客戶已入單**
   - 匯入完成後，回覆客戶訂單已建立或已入單。
   - 若有任何例外狀況，先說明尚未完成的原因與需要補充的資料。

## 執行原則
- 先確保資料完整，再進行匯入。
- 任何缺漏、錯誤或不確定之處，都要先停下來確認。
- 不要假設缺少的欄位內容；應向來源資料或客戶確認。
