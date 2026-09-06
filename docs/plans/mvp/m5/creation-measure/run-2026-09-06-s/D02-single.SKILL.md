---
name: shopify-excel-order-import
description: Use this skill when you receive customer orders in Excel and need to convert them into CSV, check for missing fields, import them into the Shopify admin, and then confirm the order has been entered.
---

# 目的
將客戶提供的 Excel 訂單整理成可匯入的 CSV，先檢查缺漏欄位，再匯入 Shopify 後回覆客戶已入單。

# 流程
1. **收到客戶 Excel 訂單**
   - 先確認檔案可開啟，且內容是訂單資料。
   - 若檔案格式不是 Excel，先請對方提供正確檔案或可讀取的表格格式。

2. **轉成 CSV 格式**
   - 將 Excel 內容另存或轉出為 CSV。
   - 保留原始欄位名稱與資料順序，避免在轉檔時改動訂單內容。
   - 若有多個工作表，先確認要使用哪一個工作表；通常只匯出實際訂單資料所在的工作表。

3. **檢查缺漏欄位**
   - 檢查 CSV 是否包含匯入 Shopify 所需的必要資訊。
   - 至少確認以下類型資料是否完整：
     - 顧客識別資訊
     - 商品名稱或 SKU
     - 數量
     - 收件資訊
     - 聯絡資訊
   - 若發現缺漏、格式錯誤、重複列、空白列或不一致資料，先整理修正。
   - 若缺少無法自行推斷的必要欄位，先回覆客戶補資料，不要直接匯入。

4. **匯入 Shopify 後台**
   - 使用 Shopify 後台的訂單匯入流程，將整理好的 CSV 匯入。
   - 匯入前再次確認欄位對應正確，避免把資料寫入錯誤欄位。
   - 若系統回報錯誤，依錯誤訊息修正 CSV 後重新匯入。
   - 若無法完成匯入，記錄原因並停止，不要假設已成功。

5. **回覆客戶已入單**
   - 匯入成功後，回覆客戶訂單已建立或已入單。
   - 若有任何例外狀況，例如部分資料缺漏、部分列未匯入或需要補件，也要一併告知。

# 注意事項
- 轉檔前後都要保留原始資料備份。
- 不要在未確認資料完整前直接匯入。
- 若 Shopify 匯入格式與客戶 Excel 欄位不一致，先做欄位對照再匯入。
- 若此環境無法實際登入 Shopify 後台或沒有匯入權限，則只能完成前置整理與檢查，並明確說明無法代為完成最後匯入。
