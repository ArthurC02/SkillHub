---
name: shopify-order-intake
description: 整理及驗證客戶提供的 Excel／CSV 訂單，產生 Shopify 訂單建立資料，並在可用且獲授權時執行匯入。收到客戶訂單表格、需要檢查缺漏、準備 Shopify payload 或確認匯入結果時使用。
---

依下列五個具名節點依序完成整個流程。除非訂單輸入本身完全未提供或無法讀取，否則不要向使用者追問；採用下述安全預設並在輸出中明示。一次執行應完成所有可完成的處理，不把建立 payload 說成已成功匯入，也不得顯示、保存或回傳任何密鑰或憑證。

## 開始：收到客戶 Excel 訂單

1. 接收 Excel、CSV，或直接貼在訊息中的表格資料。
2. 同時辨識使用者要求的是 `dry run` 還是實際匯入。使用者未明確要求實際匯入時，採用 `dry run`，並在最終輸出中說明此預設。
3. 保留來源列順序與來源列號，以便每個正規化商品列都能追溯到原始資料。
4. 若輸入完全沒有訂單資料，或 Excel 內容無法由目前環境讀取，僅在此情況請使用者提供可讀取的活頁簿、CSV 或直接貼上的資料；不要假裝已讀取檔案或繼續產生訂單。

## 處理：將 Excel 訂單轉成 CSV 格式

1. 若輸入為 Excel，讀取包含訂單資料的工作表，將標題列與資料列轉成 CSV；流程圖未指定工作表選擇規則，因此應使用明顯包含訂單欄位的工作表，並在處理摘要中列出實際使用的工作表。若無法可靠辨識，將此事列為阻擋問題，不臆測資料。
2. 若輸入已是 CSV 或貼上的逗號分隔資料，直接把它視為本節點的 CSV 結果，不要求重傳 Excel。
3. 將可辨識的欄位映射至下列正規名稱：
   - `external_order_id`
   - `customer_email`
   - `sku`
   - `product_name`
   - `quantity`
   - `unit_price`
   - `currency`
   - `financial_status`
   - `fulfillment_status`
   - `shipping_name`
   - `shipping_address1`
   - `shipping_city`
   - `shipping_postal_code`
   - `shipping_country`
4. 以 CSV 規則正確處理逗號、引號、換行與 UTF-8 文字，不因中文內容破壞欄位。
5. 以 `external_order_id` 合併同一訂單的多個商品列，但保留每個商品項目及其來源列。不得把同一訂單的商品合計成單一商品列。

## 處理：檢查缺漏欄位

1. 對每個商品列檢查以下必填資料：
   - `external_order_id`
   - `customer_email`
   - `sku` 或 `product_name` 至少一項
   - `quantity`，且必須是正整數
   - `unit_price`，且必須是非負數
   - `shipping_name`
   - `shipping_address1`
   - `shipping_city`
   - `shipping_postal_code`
   - `shipping_country`
2. 對缺值套用且只套用以下安全預設：
   - `currency=TWD`
   - `financial_status=pending`
   - `fulfillment_status=unfulfilled`
3. 在驗證報告逐項列出套用預設的欄位、值及受影響的訂單或來源列。不得臆造缺少的客戶識別、商品、數量、價格或地址。
4. 同一 `external_order_id` 的訂單層級資料若互相衝突，例如客戶 email、幣別或收件地址不同，將衝突列為阻擋錯誤並指出來源列，不自行選定其中一值。
5. 對每筆訂單計算商品總額：`sum(quantity × unit_price)`。不得將運費、稅額或折扣加入商品總額，因流程圖與輸入未定義這些值。
6. 產生驗證報告，至少包含：檢查的訂單數、商品列數、套用的預設、阻擋錯誤及計算後的各訂單商品總額。沒有阻擋錯誤時，明確寫出「沒有阻擋建立訂單的必填欄位錯誤」。
7. 產生正規化 CSV，固定使用本節點列出的正規欄位順序，並附加 `source_row` 供追溯。相同 `external_order_id` 的每個商品仍各占一列。

## 處理：將資料匯入 Shopify 後台

1. 為每個通過驗證的 `external_order_id` 建立一個結構化 JSON 訂單物件；同一訂單的商品放入同一個 `line_items` 陣列。JSON 至少包含：
   - `external_order_id`
   - `email`
   - `currency`
   - `financial_status`
   - `fulfillment_status`
   - `line_items`，每項含 `sku`、`title`、`quantity`、`price` 與 `source_row`
   - `shipping_address`，含 `name`、`address1`、`city`、`zip` 與 `country_code`
   - `total_line_items_price`
2. 最外層使用 `orders` 陣列；每個通過驗證的外部訂單只出現一次。保留數值型別，使 `quantity` 為整數、價格與總額為數值。
3. 若存在阻擋錯誤，不得提交受影響訂單；仍應輸出可供修正的正規化 CSV、驗證報告與 JSON 草稿，並把執行狀態標示為未匯入。
4. 若本次是 `dry run`，只產生 JSON，不呼叫外部匯入介面，並將狀態明確標示為 `dry run — 尚未匯入 Shopify`。
5. 實際匯入只有在使用者明確要求，且目前環境確實具有可用、獲授權的 Shopify Admin API 能力時才執行。流程圖未指定 API 介面、版本或呼叫方式，因此使用環境中已獲授權的 Shopify 訂單建立介面，不虛構端點、工具或成功結果。
6. 只有收到 Shopify 明確的成功結果及訂單識別碼後，才把該訂單標示為已匯入。記錄可安全輸出的 Shopify 訂單識別碼；不得輸出憑證、權杖或其他秘密。
7. 若沒有執行能力、沒有成功回應，或 Shopify 回傳錯誤，狀態必須是尚未匯入或匯入失敗，並如實列出已知原因，不得聲稱已建立訂單。

## 結束：回覆客戶已入單

以固定順序輸出以下六個區塊，在一次回覆中完成所有結果：

1. **處理摘要**：列出來源格式、訂單數、商品列數、如何依 `external_order_id` 合併，以及本次採用的模式。
2. **驗證報告**：列出必填檢查、阻擋錯誤、所有已套用預設及每筆訂單商品總額。
3. **正規化 CSV**：放在 `csv` 程式碼區塊中，包含所有正規欄位與 `source_row`。
4. **Shopify 訂單建立 JSON**：放在 `json` 程式碼區塊中，使用 `orders` 陣列，即使本次未實際匯入也要提供。
5. **執行狀態**：逐筆列出 `external_order_id`、`dry run`／尚未匯入／匯入失敗／已匯入，以及僅在成功時列出的 Shopify 訂單識別碼。
6. **客戶回覆草稿**：
   - 只有 Shopify 已回傳成功及訂單識別碼時，才可寫資料已入單，並可附安全的訂單識別碼。
   - dry run 或尚未實際匯入時，說明資料已整理與檢查；若驗證通過，明確寫「資料已整理並通過檢查，目前尚待實際匯入」，不得寫成已入單。
   - 有阻擋錯誤時，簡潔列出需要補正的欄位或衝突，並明確說明尚未匯入。

最終回覆使用使用者所用語言。不要以問題結尾；只要輸入資料存在，就直接提供上述可完成的結果與明確狀態。