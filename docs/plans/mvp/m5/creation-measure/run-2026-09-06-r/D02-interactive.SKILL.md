---
name: excel-to-shopify-order-flow
description: 將客戶 Excel 訂單依流程轉成 CSV、檢查缺漏欄位、匯入 Shopify 後台，並回覆客戶已入單；用在需要把已確認的訂單處理流程整理成可執行 Skill 時。
---

# Excel 訂單轉 Shopify 流程

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Purpose

When given a customer Excel order, follow the confirmed flow in order: receive the order, convert it to CSV, check for missing fields, import it into the Shopify admin, and reply that the order has been entered.

## Steps

1. **收到客戶 Excel 訂單**
   - Read the input order exactly as provided.
   - If the input is silent about a needed detail, write `not given`.

2. **轉成 CSV 格式**
   - Convert the order content into CSV using only fields present in the input.
   - Do not add new columns, rows, assumptions, or extra values.
   - If a field is missing in the input, write `not given` for that field.

3. **檢查缺漏欄位**
   - Check whether any required order fields are missing from the input.
   - Report missing fields plainly.
   - Do not invent replacements.

4. **匯入 Shopify 後台**
   - Prepare the CSV content for Shopify admin import.
   - If the input does not provide a specific Shopify import requirement, write `not given`.
   - Do not describe extra import steps beyond the confirmed flow.

5. **回覆客戶已入單**
   - Produce the customer reply stating that the order has been entered.
   - Keep the reply grounded in the input and the confirmed flow.

## Output

Return the finished artifact itself: the CSV-ready order handling result and the customer reply.

If the input is missing information needed to complete the artifact, write `not given` for that item instead of asking a question.

## Constraints

- Follow the confirmed node order exactly.
- Do not add any step, branch, or condition that is not present in the confirmed flow.
- Use only what the input contains.
- Write `not given` wherever the input is silent.