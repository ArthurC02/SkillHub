---
name: shopify-excel-order-import
description: Use this skill when you receive customer orders in Excel and need to convert them into CSV, check for missing fields, import them into the Shopify admin, and then confirm the order has been entered.
---

# Excel 訂單匯入 Shopify 流程

依照圖中的流程執行：收到客戶 Excel 訂單 → 轉成 CSV 格式 → 檢查缺漏欄位 → 匯入 Shopify 後台 → 回覆客戶已入單。

## 執行步驟

1. **確認收到的 Excel 訂單內容**
   - 先確認檔案可開啟，且是客戶提供的訂單資料。
   - 檢查工作表名稱、欄位標題、資料列是否完整可讀。

2. **轉成 CSV 格式**
   - 將 Excel 另存或匯出為 CSV。
   - 優先使用與 Shopify 匯入需求相容的編碼與分隔格式。
   - 保留原始檔作為備份，不直接覆蓋。

3. **檢查缺漏欄位**
   - 檢查必要欄位是否存在且有值，例如：客戶資訊、商品資訊、數量、地址或其他匯入所需欄位。
   - 檢查常見問題：空白列、欄位名稱不一致、日期格式錯誤、數量不是數字、商品名稱拼寫不一致。
   - 若有缺漏或格式不符，先整理修正；若無法判定，回頭向提供訂單的人確認，不要直接匯入不完整資料。

4. **匯入 Shopify 後台**
   - 使用 Shopify 後台的對應匯入功能，將整理好的 CSV 上傳。
   - 匯入前再次確認欄位對應正確，避免資料進錯欄。
   - 匯入後檢查系統回饋，確認是否成功或是否有錯誤列。

5. **確認已入單**
   - 驗證訂單是否已成功建立或出現在 Shopify 後台。
   - 若有失敗項目，記錄失敗原因並修正後重新匯入。
   - 完成後回覆客戶：訂單已入單。

## 注意事項

- 不要在未檢查缺漏欄位前直接匯入。
- 若 CSV 格式與 Shopify 要求不一致，先修正格式再匯入。
- 若缺少必要資訊且無法自行補齊，應先請客戶補資料。
