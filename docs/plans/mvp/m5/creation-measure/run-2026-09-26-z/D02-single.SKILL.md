---
name: shopify-excel-order-import
description: 'Use when you need to process customer Excel orders into Shopify: convert the spreadsheet to CSV, check for missing fields, import into the Shopify admin, and then report back that the order has been entered.'
---

# Shopify Excel 訂單匯入流程

依照流程圖執行以下步驟，將客戶提供的 Excel 訂單整理後匯入 Shopify，並回覆客戶已入單。

## 1. 收到客戶 Excel 訂單
- 先確認收到的是客戶訂單檔案，且格式為 Excel（例如 `.xlsx` 或 `.xls`）。
- 若檔案無法開啟、不是 Excel、或內容明顯損毀，先回覆客戶請重新提供可讀取的檔案。

## 2. 轉成 CSV 格式
- 將 Excel 內容另存或匯出為 CSV。
- 匯出前先確認工作表是否為正確的訂單頁。
- 若有多個工作表，只使用包含訂單資料的那一頁。
- 匯出後檢查 CSV 是否保留了所有必要欄位與資料列。

## 3. 檢查缺漏欄位
- 檢查每筆訂單資料是否缺少必要資訊。
- 至少確認以下常見欄位是否完整：
  - 客戶姓名
  - 聯絡方式
  - 商品名稱或 SKU
  - 數量
  - 收件資訊
  - 其他 Shopify 匯入所需欄位
- 若發現缺漏：
  - 先整理出缺少的欄位與對應列號。
  - 不要自行猜測關鍵資料。
  - 回覆客戶補齊後再進行匯入，或在可行時依既有資料補正。

## 4. 匯入 Shopify 後台
- 將整理好的 CSV 匯入 Shopify 後台對應的訂單或資料匯入流程。
- 匯入前再次確認欄位對應正確，避免資料錯置。
- 匯入後檢查結果是否成功：
  - 是否建立成功
  - 是否有錯誤訊息
  - 是否有部分列失敗
- 若有錯誤，依錯誤訊息修正 CSV 後重新匯入。

## 5. 回覆客戶已入單
- 確認匯入成功後，回覆客戶訂單已完成入單。
- 若有未能匯入的資料，清楚告知缺漏項目與需要補充的內容。
- 回覆內容應簡潔明確，包含：
  - 已完成匯入
  - 若有例外，說明哪些資料仍需補件

## 6. 注意事項
- 不要在未檢查缺漏欄位前直接匯入。
- 不要自行補寫不確定的客戶資料。
- 若 Shopify 後台無法存取、沒有登入權限，則只能整理 CSV 與缺漏清單，並請有權限的人員完成匯入。
