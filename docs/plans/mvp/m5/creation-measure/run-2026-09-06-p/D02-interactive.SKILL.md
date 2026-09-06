---
name: shopify-order-csv-import-reply
description: Convert customer-provided Excel order data into a Shopify-importable CSV, check for missing fields, and draft a completion reply when you need a reusable skill for order intake workflows.
---

# Shopify order CSV import and reply

Use this skill when you receive customer Excel order data that needs to be turned into a Shopify-importable CSV, checked for missing fields, and followed by a customer completion reply.

## Instructions

1. **收到客戶 Excel 訂單**
   - Take the Excel order data exactly as provided by the user.
   - If the input is missing the order data itself, stop and say what is missing.
   - Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

2. **轉成 CSV 格式**
   - Convert the provided rows into CSV using only the columns and values present in the input.
   - If the user has supplied field-mapping rules, apply only those rules.
   - If a value is absent in the input, keep it as not given rather than inventing it.

3. **檢查缺漏欄位**
   - Compare the provided data against any field rules included in the input.
   - List missing fields that are actually absent from the supplied material.
   - If no field rules are provided, say that the missing-field rule is not given.

4. **匯入 Shopify 後台**
   - Prepare the CSV content in the shape needed for Shopify import only when the input gives enough information to do so.
   - If Shopify-specific column mapping or import requirements are not given, state that they are not given.

5. **回覆客戶已入單**
   - Draft a short completion reply for the customer based only on the information in the input.
   - Do not claim the order was imported unless the input explicitly says it was completed.
   - Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output

Return the finished artifact in a clear Markdown structure with these parts:
- Conversion result
- Missing-field check
- Customer reply

If the input does not provide enough information for a section, write not given.