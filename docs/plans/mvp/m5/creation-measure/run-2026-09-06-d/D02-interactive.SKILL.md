---
name: excel-to-shopify-order-intake
description: Use this skill when you need to turn a customer’s Excel order into CSV, check for missing fields, import the order into Shopify, and send a confirmation that the order has been entered.
---

# Excel to Shopify Order Intake

## Purpose
Use this skill when a customer order arrives in Excel and must be converted into CSV, checked for missing fields, imported into Shopify, and acknowledged as entered.

## Workflow
1. Open the customer Excel order and identify the order rows and required fields.
2. Convert the Excel data into CSV format.
3. Check the CSV for missing required fields before import.
4. Import the CSV data into Shopify admin.
5. Send a confirmation message to the customer stating that the order has been entered.

## Required inputs
- The customer’s Excel order file.
- The list of required fields for the order format in use.
- Access to Shopify admin if the import step is to be completed.
- A channel for sending the customer confirmation.

## Output
- A CSV version of the order data.
- A completed Shopify import outcome.
- A customer confirmation that the order has been entered.

## Rules
- Do not skip the missing-field check before import.
- Do not invent missing data. If required fields are missing, stop and flag the issue for correction before continuing.
- Do not add extra branches or fallback paths beyond the stated workflow.
- If Shopify import access is unavailable, report that the workflow cannot be completed.
- If the confirmation channel is unavailable, report that the customer cannot yet be notified.

## Notes
- This skill assumes a straightforward linear workflow.
- Any exceptions should be handled as a limitation to report, not as a new process branch.