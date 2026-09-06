---
name: excel-to-shopify-order-flow
description: 將客戶 Excel 訂單依流程轉成 CSV、檢查缺漏欄位、匯入 Shopify 後台，並回覆客戶已入單；用在需要把已確認的訂單處理流程整理成可執行 Skill 時。
---

# Excel 訂單轉 Shopify 流程

## Purpose

When given a customer Excel order, follow the confirmed flow in order: receive the order, convert it to CSV, check for missing fields, import it into the Shopify admin, and reply that the order has been entered.

## Required response shape

Return the finished artifact itself. The output must include all of the following, in this order:

1. A short 3-step procedure for converting the Excel order to CSV, and one of the steps must explicitly say `轉存為 CSV`.
2. A missing-field check that explicitly says `檢查缺漏欄位` and reports whether the sample input is complete or which field is missing.
3. A Shopify admin import note that explicitly says `匯入 Shopify 後台`.
4. A customer reply that explicitly says `回覆客戶已入單`.

## Operating rules

- Use only what the input contains.
- Do not add any tool, service, or import method that is not in the confirmed flow.
- Do not mention Matrixify, API, or any other extra import approach.
- Do not add new steps, branches, or conditions.
- If the input is silent about a detail, write `not given` instead of inventing it.

## Hard limit

- Output exactly these four steps and no others: 1) 轉成 CSV 格式, 2) 檢查缺漏欄位, 3) 匯入 Shopify 後台, 4) 回覆客戶已入單.
- Do not mention any other tool, service, workflow, or alternative import method.
- Explicitly forbid Matrixify, API, automation tools, draft orders, and any similar workaround.

## Procedure to follow

1. **收到客戶 Excel 訂單**
   - Read the input order exactly as provided.
   - If the input is silent about a needed detail, write `not given`.

2. **轉成 CSV 格式**
   - Convert the order content into CSV using only fields present in the input.
   - The explanation for this step must explicitly include `轉存為 CSV`.
   - Do not add new columns, rows, assumptions, or extra values.

3. **檢查缺漏欄位**
   - Check whether any required order fields are missing from the input.
   - State whether the sample input is complete or name the missing field.
   - Do not invent replacements.

4. **匯入 Shopify 後台**
   - State only that the CSV is ready for Shopify admin import; do not suggest any other import route.

5. **回覆客戶已入單**
   - Produce the customer reply stating that the order has been entered.
   - Keep the reply grounded in the input and the confirmed flow.

## Constraints

- Follow the confirmed node order exactly.
- Do not add any step, branch, or condition that is not present in the confirmed flow.
- Use only what the input contains.
- Write `not given` wherever the input is silent.