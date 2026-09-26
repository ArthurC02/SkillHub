---
name: excel-to-shopify-order-import
description: Use this skill when you need to process a customer Excel order into a CSV for Shopify import, check for missing fields, and prepare the customer reply confirming the order was entered.
---

# Excel 訂單轉 Shopify 匯入流程

依照流程圖執行：收到客戶 Excel 訂單後，先轉成 CSV，再檢查缺漏欄位，接著匯入 Shopify 後台，最後回覆客戶已入單。

## 執行步驟

1. **收到客戶 Excel 訂單**
   - 確認收到的是可讀取的 Excel 檔案。
   - 先快速確認檔案內容是否為訂單資料，而不是其他附件。

2. **轉成 CSV 格式**
   - 將 Excel 內容轉存為 CSV。
   - 保留原始欄位名稱與資料順序，避免在轉檔時改動欄位意義。
   - 若有多個工作表，先確認哪一個是要匯入的訂單表。

3. **檢查缺漏欄位**
   - 檢查匯入 Shopify 所需的必要欄位是否完整。
   - 特別留意常見缺漏：商品名稱、數量、規格、收件資訊、聯絡方式、地址、郵遞區號、備註等。
   - 若發現缺漏或格式不一致，先整理成待確認清單，不要直接匯入。
   - 若資料可補齊，先補齊後再進行下一步；若無法補齊，先向客戶確認。

4. **匯入 Shopify 後台**
   - 使用整理好的 CSV 進行 Shopify 後台匯入。
   - 匯入前再次確認欄位對應正確，避免資料進錯欄。
   - 匯入後檢查是否成功建立訂單或是否有錯誤訊息。
   - 若匯入失敗，根據錯誤訊息回到前一步修正 CSV。

5. **回覆客戶已入單**
   - 在確認匯入成功後，回覆客戶訂單已建立或已入單。
   - 若仍有缺漏或匯入問題，回覆客戶需要補充的資訊與原因。

## 注意事項

- 不要假設缺漏欄位已存在；每次都要實際檢查。
- 不要在未確認欄位完整前直接匯入。
- 若流程中任何一步無法完成，先停下來整理問題，再決定是否需要向客戶確認。
