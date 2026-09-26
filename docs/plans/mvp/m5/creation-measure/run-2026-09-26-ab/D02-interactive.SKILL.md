---
name: shopify-order-csv-import
description: 將客戶 Excel 訂單轉成 CSV、檢查缺漏欄位，並依流程匯入 Shopify 後台後回覆已入單；當你要把確認過的訂單資料整理成可匯入格式並產出完成通知時使用。
---

# Purpose
This skill turns a customer Excel order into CSV, checks for missing fields, imports it into Shopify admin, and returns a completion message.

# Instructions
1. 收到客戶 Excel 訂單
   - Read the order data exactly as provided.
   - If the input does not contain the order content, stop and ask for it.

2. 轉成 CSV 格式
   - Convert the provided Excel order data into CSV.
   - Preserve only the columns and values present in the input.

3. 檢查缺漏欄位
   - Check whether any required fields are missing in the provided order data.
   - Report the missing fields if any are present.

4. 匯入 Shopify 後台
   - Prepare the CSV for Shopify admin import.
   - Proceed with the Shopify import step.

5. 回覆客戶已入單
   - Produce a customer-facing message stating the order has been entered.

# Constraints
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- Follow the diagram in order and do not add any step, condition, role, or tool that is not shown.
- Where the diagram is silent, say so instead of inventing details.
- Do not introduce extra branches, retries, escalation paths, or補件 workflows.
- If required information is missing from the input, ask only for that missing information.

# Output
Return the CSV content, the missing-field check result, and the customer reply message.