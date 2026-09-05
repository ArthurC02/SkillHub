---
name: shopify-excel-order-import
description: Use when you need to process customer Excel orders into a Shopify-ready CSV, check for missing fields, and prepare the order response workflow shown in the diagram.
---

# Shopify Excel 訂單處理流程

依照圖中的流程，將收到的客戶 Excel 訂單轉成可匯入 Shopify 的 CSV，檢查缺漏欄位，完成匯入後回覆客戶已入單。

## 流程步驟

1. **收到客戶 Excel 訂單**
   - 先確認檔案可開啟，並辨識這份檔案是否為客戶訂單資料。
   - 若檔案無法讀取或不是訂單格式，先回覆需要可用的 Excel 檔。

2. **轉成 CSV 格式**
   - 將 Excel 內容整理成 Shopify 可接受的 CSV 結構。
   - 保留必要欄位，並確保欄位名稱、資料格式與 Shopify 匯入需求一致。
   - 若原始 Excel 有多個工作表，只使用與訂單相關的資料。

3. **檢查缺漏欄位**
   - 檢查每筆訂單是否缺少必要資訊，例如商品、數量、收件資訊、聯絡方式或其他匯入所需欄位。
   - 若發現缺漏：
     - 先整理出缺少的欄位與受影響的訂單列。
     - 不要直接假設缺值內容。
     - 需要時向客戶確認補件後再繼續。
   - 若欄位完整，繼續下一步。

4. **匯入 Shopify 後台**
   - 使用整理好的 CSV 進行 Shopify 後台匯入。
   - 匯入前再次確認欄位對應正確，避免資料錯位。
   - 若匯入失敗，根據錯誤訊息回頭檢查 CSV 格式或缺漏欄位。

5. **回覆客戶已入單**
   - 匯入成功後，通知客戶訂單已完成入單。
   - 若有任何缺漏、修正或匯入限制，也一併簡潔說明。

## 執行原則

- 先轉檔，再檢查，再匯入，最後回覆。
- 不要跳過缺漏欄位檢查。
- 不要在未確認資料時自行補值。
- 若流程中任何一步失敗，先處理該步驟的問題，再往下進行。
