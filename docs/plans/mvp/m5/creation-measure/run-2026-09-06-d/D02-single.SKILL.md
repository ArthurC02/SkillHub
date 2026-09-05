---
name: shopify-excel-order-import
description: Use when you need to process a customer Excel order file into a Shopify-ready CSV, check for missing fields, and prepare the order update response. The flow is triggered by receiving an Excel order from a customer and ends with replying that the order has been entered.
---

# 目的
將客戶提供的 Excel 訂單轉成可匯入 Shopify 的 CSV，檢查缺漏欄位，完成匯入準備後回覆客戶已入單。

# 適用時機
當任務是「收到客戶 Excel 訂單 → 轉 CSV → 檢查欄位 → 匯入 Shopify 後台 → 回覆客戶」時使用本技能。

# 執行流程
1. **收到客戶 Excel 訂單**
   - 先確認檔案可開啟，且內容確實是訂單資料。
   - 若檔案不是 Excel、內容無法辨識，先請求可讀取的原始檔案。

2. **轉成 CSV 格式**
   - 將 Excel 內容整理成 CSV 可用的欄位結構。
   - 保留必要的訂單資訊，例如商品、數量、規格、收件資訊、聯絡資訊等，依實際檔案欄位對應。
   - 確保文字編碼與分隔格式適合後續匯入。

3. **檢查缺漏欄位**
   - 檢查每筆訂單是否有缺少必要欄位。
   - 常見需確認的項目包括：商品名稱或 SKU、數量、收件人姓名、地址、電話、Email、配送方式、備註。
   - 若發現缺漏：
     - 先標記缺漏項目。
     - 若可從其他欄位推得，補齊後再確認一次。
     - 若無法推得，整理成待補資料清單，回覆客戶補件。

4. **匯入 Shopify 後台**
   - 將整理好的 CSV 準備為可匯入格式。
   - 依 Shopify 匯入需求確認欄位名稱、格式與資料完整性。
   - 若匯入前仍有不符合格式之處，先修正再進行匯入準備。
   - 若此環境無法實際登入或操作 Shopify 後台，明確說明無法直接完成登入匯入，只能提供匯入前檢查與 CSV 整理結果。

5. **回覆客戶已入單**
   - 在確認資料已完成整理並可匯入後，回覆客戶訂單已處理。
   - 若有缺漏或需補件，則不要直接宣稱已完成入單；應先說明缺少哪些資料。

# 輸出原則
- 以訂單處理為主，不自行新增不存在的欄位。
- 遇到缺漏時，優先列出問題與需要補充的資訊。
- 若無法實際執行 Shopify 後台操作，清楚說明限制，並提供已完成的 CSV 整理與檢查結果。
