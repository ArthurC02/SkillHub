---
name: shopify-excel-order-import
description: Use this skill when you receive customer orders in Excel and need to convert them into a CSV, check for missing fields, import them into the Shopify admin, and then confirm the order has been entered.
---

# Excel 訂單轉 CSV 並匯入 Shopify

依照下列流程處理客戶 Excel 訂單：

1. **收到客戶 Excel 訂單**
   - 先確認檔案可開啟，且內容是訂單資料。
   - 若檔案格式不是 Excel，先請對方提供可匯入的表格檔。

2. **轉成 CSV 格式**
   - 將 Excel 另存或匯出為 CSV。
   - 保留欄位名稱與資料順序，避免在轉檔時改動內容。
   - 若有多個工作表，先確認要匯入的是哪一個工作表。

3. **檢查缺漏欄位**
   - 檢查 CSV 是否包含匯入 Shopify 所需的必要欄位。
   - 找出空白、格式錯誤、重複或不一致的資料。
   - 若有缺漏欄位，先補齊或回覆客戶確認，不要直接匯入。
   - 匯入前再次確認數量、品項、收件資訊與其他關鍵欄位正確。

4. **匯入 Shopify 後台**
   - 將整理好的 CSV 匯入 Shopify 後台對應的訂單流程。
   - 匯入後檢查系統回饋，確認沒有錯誤或失敗項目。
   - 若有失敗列，記錄原因並修正後重新匯入。

5. **回覆客戶已入單**
   - 確認訂單已成功建立後，再回覆客戶已完成入單。
   - 若有任何缺漏、錯誤或無法匯入的項目，先說明原因並請對方補資料。

## 注意事項
- 這個流程需要可存取檔案與 Shopify 後台；若沒有後台權限或檔案無法讀取，先停止並請求必要資訊。
- 不要在未檢查缺漏欄位前直接匯入。
- 保留原始 Excel 與轉出的 CSV，方便追蹤與修正。
