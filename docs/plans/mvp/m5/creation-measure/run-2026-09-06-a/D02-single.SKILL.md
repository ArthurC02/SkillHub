---
name: shopify-excel-order-import
description: Use when you need to process a customer Excel order file into a CSV, check for missing fields, and prepare it for Shopify backend import before replying that the order has been entered.
---

# Shopify Excel 訂單匯入流程

依照流程圖執行：收到客戶 Excel 訂單後，先轉成 CSV，再檢查缺漏欄位，接著匯入 Shopify 後台，最後回覆客戶已入單。

## 執行步驟

1. **收到客戶 Excel 訂單**
   - 確認收到的是可讀取的 Excel 檔案。
   - 若檔案無法開啟或格式異常，先請客戶重新提供可用檔案。

2. **轉成 CSV 格式**
   - 將 Excel 內容另存或匯出為 CSV。
   - 保留原始欄位名稱與資料順序，避免在轉檔時改動內容。
   - 若有多個工作表，先確認哪一個工作表是訂單資料來源，再進行轉檔。

3. **檢查缺漏欄位**
   - 檢查 CSV 中是否有必要欄位缺值、空白列、格式錯誤或不一致資料。
   - 常見需確認的內容包括：商品、數量、規格、收件資訊、聯絡方式、地址等。
   - 若發現缺漏或不清楚的資料，先整理成問題清單，回頭向客戶確認後再繼續。
   - 若資料完整，才進入下一步。

4. **匯入 Shopify 後台**
   - 將整理好的 CSV 依 Shopify 匯入需求上傳或建立訂單。
   - 匯入後檢查是否成功建立訂單、商品與數量是否正確、收件資訊是否完整。
   - 若匯入失敗，根據錯誤訊息修正 CSV 或資料後重新匯入。

5. **回覆客戶已入單**
   - 確認訂單已成功建立後，再通知客戶已完成入單。
   - 若仍有待確認項目，不要直接回覆已入單；應先說明缺少哪些資料並等待補件。

## 注意事項

- 在匯入前先做資料檢查，避免把缺漏資料直接送進 Shopify。
- 若流程中任何一步失敗，先修正資料再往下走，不要跳步。
- 這個技能只涵蓋整理、檢查與匯入前後的作業；若需要實際登入特定系統或執行受限操作，應依可用權限處理。
