---
name: excel-order-to-shopify-skill
description: Use this skill when you need to turn a customer Excel order into a Shopify-ready processing flow, with the fixed sequence of converting to CSV, checking missing fields, importing into Shopify, and replying that the order has been entered.
---

# Goal
Turn the provided customer Excel order into a portable Agent Skill that follows the confirmed flow exactly.

## Rules
- use only what the input contains — never add a date, name, assumption, step, condition, or branch the input does not give.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Steps
1. 收到客戶 Excel 訂單
   - Take the order content exactly as provided.

2. 轉成 CSV 格式
   - Convert the provided order data into CSV form.
   - Keep the structure faithful to the input and do not add columns or rows that are not given.

3. 檢查缺漏欄位
   - Review the CSV for missing fields that are visible from the provided input.

4. 匯入 Shopify 後台
   - Prepare the processed CSV for Shopify import as described by the input.

5. 回覆客戶已入單
   - Produce the final reply stating the order has been entered.

## Output
Return the finished Skill artifact itself, not a summary of how to create it.

## Constraints
- Follow the five nodes in order.
- Do not add any step, condition, role, or branch that is not shown in the confirmed flow.
- Do not invent any missing information; leave it unspecified.