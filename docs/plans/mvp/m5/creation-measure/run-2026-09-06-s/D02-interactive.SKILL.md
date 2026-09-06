---
name: shopify-order-csv-prep
description: Converts customer Excel order data into Shopify-importable CSV and a short completion summary. Use when you receive order tables that need to be prepared for Shopify import and reported back in one pass.
---

# Purpose
Turn the customer Excel order data you are given into a Shopify-importable CSV and a short completion summary.

# Use this skill when
Use this skill when the input is customer order table data that needs to be prepared for Shopify import and reported as finished in the same run.

# Instructions
1. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
2. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
3. Follow the diagram nodes in order:
   - 收到客戶 Excel 訂單
   - 轉成 CSV 格式
   - 檢查缺漏欄位
   - 匯入 Shopify 後台
   - 回覆客戶已入單
4. At 收到客戶 Excel 訂單, take the order table exactly as provided.
5. At 轉成 CSV 格式, convert the provided table into CSV-formatted output using only columns and values present in the input.
6. At 檢查缺漏欄位, inspect the provided rows for missing fields and mark missing values as 'not given'.
7. At 匯入 Shopify 後台, produce the Shopify-importable result from the converted CSV content. If Shopify-specific mapping is not given, keep the output limited to the provided columns and state that Shopify field mapping is not given.
8. At 回覆客戶已入單, include a brief completion summary stating that the order data has been prepared and whether any fields were marked 'not given'.
9. Do not add any extra branch, exception path, retry logic, or failure handling that is not present in the confirmed diagram.
10. If the input does not include enough table data to proceed, say what is missing and stop.

# Output
Return the finished artifact itself with:
- a CSV block or CSV-formatted section
- a short summary
- any missing fields clearly labeled as 'not given'

# Constraints
- Do not invent columns, values, or Shopify rules that are not in the input.
- Do not ask follow-up questions unless the input itself is incomplete.
- Keep the output limited to the confirmed flow and the material supplied by the user.