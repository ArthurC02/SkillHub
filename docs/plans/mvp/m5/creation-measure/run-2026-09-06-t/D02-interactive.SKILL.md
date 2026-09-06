---
name: excel-order-to-shopify-flow
description: 將客戶 Excel 訂單整理成 CSV、檢查缺漏欄位、匯入 Shopify 後台並回覆客戶已入單；適用於要把這個已確認流程寫成可執行技能時。
---

# Purpose
把使用者提供的流程文字，整理成可執行的流程說明。

# Instructions
1. 只根據輸入內容整理流程，依序走完已確認的節點：
   - 收到客戶 Excel 訂單
   - 轉成 CSV 格式
   - 檢查缺漏欄位
   - 匯入 Shopify 後台
   - 回覆客戶已入單
2. 保持節點順序，不要跳步，也不要新增流程圖沒有出現的步驟、分支、條件或角色。
3. 如果流程圖或輸入沒有說明某一步的細節，就直接寫「not given」。
4. 只輸出完成後的流程整理結果，不要說明規則、不要寫計畫、不要反問使用者。
5. 若輸入本身缺少必要資訊，才指出缺少之處；否則不要自行補假設。
6. 輸出格式用 Markdown 條列，讓人可以直接閱讀。

# Verbatim rules
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access

# Output
- 以 Markdown 條列輸出整理後的流程。
- 保留「檢查缺漏欄位」這一步；若缺少處理方式，寫 not given。
- 明確保留輸入來源為客戶 Excel 訂單、匯入目標為 Shopify 後台、以及最後回覆客戶已入單。