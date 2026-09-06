---
name: shopify-excel-order-import
description: Use this skill when you receive customer orders in Excel and need to convert them into CSV, check for missing fields, import them into the Shopify admin, and then confirm the order has been entered.
---

# Excel 訂單匯入 Shopify 流程

依照圖中的流程執行：收到客戶 Excel 訂單 → 轉成 CSV 格式 → 檢查缺漏欄位 → 匯入 Shopify 後台 → 回覆客戶已入單。

## 執行步驟

1. **接收並確認 Excel 訂單**
   - 先確認檔案可開啟，且內容確實是客戶訂單。
   - 檢查工作表名稱、欄位標題、資料列數是否合理。
   - 若檔案損毀、無法讀取，或不是訂單格式，先回覆需要重新提供正確檔案。

2. **轉成 CSV 格式**
   - 將 Excel 另存或匯出為 CSV。
   - 保留原始欄位順序，避免改動欄位名稱，除非 Shopify 匯入格式明確要求調整。
   - 確認編碼與分隔符號正確，避免中文亂碼或欄位錯位。

3. **檢查缺漏欄位**
   - 逐欄檢查每筆訂單是否缺少必要資訊。
   - 常見必填資訊包含：客戶姓名、聯絡方式、商品名稱或 SKU、數量、收件資訊、付款或備註欄位中必要內容。
   - 若有缺漏：
     - 先標記缺少的欄位與對應列號。
     - 不要自行補填不確定的資料。
     - 需要時先向客戶或內部窗口確認後再繼續。
   - 若欄位名稱與 Shopify 匯入需求不一致，先整理成可匯入的欄位對應。

4. **匯入 Shopify 後台**
   - 使用 Shopify 後台的訂單建立或匯入流程，將 CSV 資料導入。
   - 匯入前再次確認：
     - 商品與 SKU 是否對應正確
     - 數量是否正確
     - 客戶資料與收件資料是否完整
     - 幣別、價格、稅務或運費欄位是否符合需求
   - 若匯入失敗，根據錯誤訊息回頭修正 CSV，再重新匯入。

5. **回覆客戶已入單**
   - 匯入成功後，確認訂單已出現在 Shopify 後台。
   - 回覆客戶或內部窗口：訂單已完成入單。
   - 若有未完成項目，清楚說明缺少哪些資料與下一步需要補件的內容。

## 注意事項

- 不要在未確認的情況下猜測缺漏資料。
- 不要修改原始 Excel 檔，除非需要保留一份處理後版本。
- 若 Shopify 匯入格式與原始 Excel 差異很大，先建立對應表再轉檔。
- 若此流程需要登入 Shopify 後台，而目前沒有可用登入權限，則只能整理 CSV、檢查欄位並準備匯入資料，無法完成實際上傳。
