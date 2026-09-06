---
name: shopify-order-csv-import
description: Use this skill when you receive customer orders in Excel and need to prepare them for Shopify import, including converting to CSV, checking for missing fields, and replying that the order has been entered.
---

# Shopify 訂單匯入流程

依照流程圖執行：收到客戶 Excel 訂單後，先轉成 CSV，檢查缺漏欄位，再匯入 Shopify 後台，最後回覆客戶已入單。

## 執行步驟

1. **接收 Excel 訂單**
   - 確認收到的是客戶提供的 Excel 訂單檔。
   - 先快速檢查檔案是否可開啟、是否有明顯損毀、是否包含訂單資料。

2. **轉成 CSV 格式**
   - 將 Excel 檔另存或匯出為 CSV。
   - 保持欄位名稱與資料內容一致，避免在轉檔時改動原始資訊。
   - 若有多個工作表，先確認哪一個工作表是要匯入的訂單資料。

3. **檢查缺漏欄位**
   - 檢查匯入 Shopify 所需的必要欄位是否齊全。
   - 特別留意常見缺漏：商品名稱、數量、價格、收件人資訊、聯絡方式、地址、SKU、備註等。
   - 若發現缺漏或格式不正確，先整理出問題清單，必要時回頭向客戶確認。
   - 在資料未補齊前，不要直接匯入。

4. **匯入 Shopify 後台**
   - 使用整理好的 CSV 進行 Shopify 後台匯入。
   - 匯入前再次確認欄位對應正確，避免資料進錯欄位。
   - 匯入後檢查結果是否成功，並確認訂單內容是否正確顯示。

5. **回覆客戶已入單**
   - 在確認匯入成功後，回覆客戶訂單已完成入單。
   - 若過程中有缺漏欄位、格式問題或匯入失敗，先說明需要補件或修正，待完成後再回覆已入單。

## 注意事項

- 這個流程需要實際操作檔案與 Shopify 後台；若你沒有檔案存取權或後台權限，先請使用者提供可用的檔案或授權。
- 不要假設欄位完整；每次都要檢查缺漏欄位。
- 匯入前保留原始 Excel 檔與轉出的 CSV 檔，方便追蹤與修正。
