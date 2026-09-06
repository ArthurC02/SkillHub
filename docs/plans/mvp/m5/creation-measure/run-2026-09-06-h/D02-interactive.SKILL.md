---
name: excel-to-shopify-order-helper
description: Converts customer Excel-style order tables into Shopify-ready CSV text, checks for missing fields, and drafts a customer-facing order confirmation. Use this when you need a conservative text-only order prep skill that does not actually log in to or import into Shopify.
---

# Excel to Shopify Order Helper

## Purpose
Follow the confirmed workflow to prepare customer order data for Shopify in a text-only, conservative way. Use this skill when a user gives an order table and wants CSV formatting, missing-field checks, and a short customer-facing status message without any real Shopify login or import.

## Inputs
- A customer order table or equivalent structured order data.
- Any field names and values the user provides.
- If the user omits important data, work only with what is present and state the omission clearly.

## Workflow
1. **收到客戶 Excel 訂單**
   - Read the order data exactly as provided.
   - Preserve the original rows and values.
   - If the input is not a table, infer the tabular structure only when the text clearly supplies it; otherwise, report that the data is insufficient for conversion.

2. **轉成 CSV 格式**
   - Convert the visible order rows into CSV text.
   - Use the columns present in the input.
   - Do not invent hidden Shopify field mappings.
   - If the user has not provided enough information to form a safe CSV, present the partial CSV that can be derived and note the missing pieces.

3. **檢查缺漏欄位**
   - Compare each row against the fields visible in the input.
   - Report any blank or missing cells that are directly observable.
   - Do not assume extra required fields unless the input itself includes them.
   - Keep the check conservative: only flag what can be confirmed from the provided data.

4. **匯入 Shopify 後台**
   - Do not perform any real login, upload, API call, or import.
   - Treat this step as a conceptual handoff only.
   - State that the skill does not actually access Shopify and only prepares the data needed for a later manual or external import.

5. **回覆客戶已入單**
   - Draft one short, customer-facing status message in the user's language.
   - Make the message reflect the actual preparation status from the input.
   - If the data is incomplete, the message should say that the order has been prepared or checked, not falsely claim completion of a real Shopify import.

## Output format
Return three sections in this order:
1. **CSV** — a fenced CSV block or plain CSV text.
2. **缺漏欄位檢查** — a concise bullet list or short table of missing/blank fields.
3. **客戶回覆文字** — one sentence the user can send to the customer.

## Constraints
- Use only the information present in the user's input.
- Do not claim to have logged in to Shopify, uploaded a file, or imported orders.
- Do not ask follow-up questions if the provided data is sufficient to proceed; instead, output the safest partial result.
- Do not add extra workflow steps beyond the five confirmed nodes.
- When the input is silent about a detail, say so rather than inventing it.

## License
MIT
