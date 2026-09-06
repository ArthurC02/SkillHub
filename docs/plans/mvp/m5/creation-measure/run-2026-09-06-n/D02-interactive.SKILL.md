---
name: excel-to-shopify-csv-order-skill
description: Turn a customer Excel order into a Shopify-importable CSV workflow and use it when the request is to document or execute the confirmed single-path order-entry process from Excel to Shopify.
---

# Purpose
Convert the confirmed single-path workflow into a usable agent skill for handling a customer Excel order and producing a Shopify-importable CSV workflow.

# Instructions
Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

Follow the workflow nodes in this exact order:

1. 收到客戶 Excel 訂單
   - Read the provided customer Excel order content.
   - If the input does not include the Excel order content, write `not given` and stop.

2. 轉成 CSV 格式
   - Convert the provided order content into CSV format using only fields present in the input.
   - If a needed field is absent from the input, write `not given` for that field.

3. 檢查缺漏欄位
   - Check the CSV content for missing fields.
   - Report missing fields exactly as missing.
   - Do not split this node into extra substeps.

4. 匯入 Shopify 後台
   - Prepare the CSV for Shopify admin import.
   - If the input does not provide import details, write `not given`.

5. 回覆客戶已入單
   - Produce the customer reply stating the order has been entered.
   - If the reply text is not provided in the input, write `not given`.

# Output
Return the finished artifact itself: the ordered workflow output with the CSV result, the missing-field check, the Shopify import-ready result, and the customer reply.

# Limits
- Do not add any branch, condition, exception flow, or extra node that is not present in the confirmed diagram.
- Do not ask follow-up questions unless the input itself is missing the material needed to do the work.
- Keep the node order unchanged.
- If any required detail is absent from the input, write `not given` instead of inventing it.