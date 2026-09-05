---
name: excel-to-shopify-order-import
description: Use this skill when you need to process a customer Excel order file into a CSV, check for missing fields, import it into the Shopify admin, and then reply to the customer that the order has been entered.
---

# Excel 訂單轉 Shopify 匯入流程

依照圖中的流程執行：收到客戶 Excel 訂單 → 轉成 CSV 格式 → 檢查缺漏欄位 → 匯入 Shopify 後台 → 回覆客戶已入單。

## 執行步驟

1. **確認收到的是客戶 Excel 訂單**
   - 先確認檔案格式是否為 Excel（例如 `.xlsx`、`.xls`）。
   - 若不是 Excel，先判斷是否仍可轉成可匯入的表格格式；若無法判斷，請先向使用者確認來源檔案內容與欄位結構。

2. **轉成 CSV 格式**
   - 將 Excel 內容另存或轉出為 CSV。
   - 保留原始欄位名稱與資料順序，避免在轉檔時改動欄位意義。
   - 若有多個工作表，先確認哪一個工作表是訂單資料，再進行轉檔。

3. **檢查缺漏欄位**
   - 檢查匯入 Shopify 所需的必要欄位是否齊全。
   - 特別留意常見缺漏：商品名稱、SKU、數量、顧客資訊、收件資訊、價格、幣別、備註等。
   - 若發現缺漏或格式不一致：
     - 先整理出缺少哪些欄位或哪些列有問題。
     - 不要直接匯入不完整資料。
     - 先請使用者補資料，或依既有規則補齊可推定的欄位。

4. **匯入 Shopify 後台**
   - 使用 Shopify 後台的訂單匯入流程，將整理好的 CSV 匯入。
   - 匯入前再次確認欄位對應是否正確，避免資料對錯欄。
   - 匯入後檢查結果是否成功，並確認訂單內容是否正確建立。
   - 若匯入失敗，記錄失敗原因並回到缺漏欄位或格式檢查步驟處理。

5. **回覆客戶已入單**
   - 在確認 Shopify 訂單已成功建立後，回覆客戶訂單已完成入單。
   - 回覆內容應簡潔明確，必要時附上訂單編號或處理狀態。

## 處理原則

- 任何欄位不清楚時，先查核再匯入，不要猜測關鍵訂單資料。
- 若 Shopify 匯入規則與客戶 Excel 欄位不一致，先做欄位映射整理。
- 若無法完成匯入，應回報卡在哪一步，以及需要客戶補充哪些資料。
- 最終目標是：資料轉成可匯入格式、缺漏已處理、訂單成功進入 Shopify、並通知客戶。
