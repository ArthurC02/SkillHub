---
name: shopify-excel-order-import
description: Use this skill when you receive customer orders in Excel and need to convert them into CSV, check for missing fields, import them into the Shopify admin, and then confirm the order has been entered.
---

# Excel 訂單匯入 Shopify 流程

依照下列流程處理客戶 Excel 訂單：

1. **收到客戶 Excel 訂單**
   - 先確認檔案可開啟，且內容是訂單資料。
   - 若檔案無法讀取、格式異常或不是 Excel，先回覆需要可用的原始訂單檔。

2. **轉成 CSV 格式**
   - 將 Excel 另存或匯出為 CSV。
   - 保留欄位名稱與資料內容，避免在轉檔時遺失資訊。
   - 若有多個工作表，先確認要匯入的是哪一張。

3. **檢查缺漏欄位**
   - 檢查 CSV 是否包含匯入 Shopify 所需的必要欄位。
   - 找出空白、格式不正確、欄位名稱不一致或資料型態錯誤的項目。
   - 若有缺漏，先整理出問題清單，必要時請補資料後再匯入。

4. **匯入 Shopify 後台**
   - 使用 Shopify 後台的訂單匯入流程，將整理好的 CSV 匯入。
   - 匯入前再次確認欄位對應正確，避免資料寫入錯誤。
   - 若系統回報錯誤，依錯誤訊息修正 CSV 後重新匯入。
   - 這一步需要可登入的 Shopify 後台；若沒有登入權限，先請使用者提供可操作的存取方式或由有權限的人執行。

5. **回覆客戶已入單**
   - 匯入成功後，確認訂單已建立或已出現在 Shopify 中。
   - 回覆客戶已完成入單，並在需要時附上訂單編號或處理結果。

## 執行原則
- 任何轉檔或匯入前，都先檢查資料完整性。
- 若發現缺漏欄位，不要直接匯入未確認的資料。
- 若無法存取 Shopify 後台，明確說明限制並請求必要權限或人工協助。
