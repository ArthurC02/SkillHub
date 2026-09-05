---
name: customer-order-excel-to-shopify
description: Processes a customer Excel order by converting it to CSV, checking for missing fields, importing it into Shopify admin, and replying that the order has been placed. Use this skill when you need a faithful, portable description of this workflow without adding unconfirmed branches or automation details.
---

# Customer Order Excel to Shopify

## Purpose
Use this skill when a customer order arrives as an Excel file and you need a portable, diagram-faithful workflow description for turning it into a CSV, checking for missing fields, importing it into Shopify admin, and replying that the order has been placed.

## Scope
This skill documents only the confirmed workflow steps:
1. Receive customer Excel order
2. Convert to CSV format
3. Check for missing fields
4. Import into Shopify admin
5. Reply to customer that the order has been placed

Do not add extra workflow branches or assume any behavior that is not shown in the diagram.

## Procedure
### 1) Receive the customer Excel order
Start with the customer-provided Excel order file. Treat this as the source input for the rest of the workflow.

### 2) Convert the Excel order to CSV
Convert the Excel data into CSV format so it can be used for the next steps in the process.

### 3) Check for missing fields
Review the CSV for missing fields.

Important: the diagram confirms only that a check happens. It does not specify which fields are required, how missing fields are handled, or whether the check causes any branch in the workflow.

### 4) Import into Shopify admin
Import the CSV into Shopify admin.

Important: the diagram does not say whether this import is manual or automated, so do not claim either one unless a later confirmed requirement says so.

### 5) Reply to the customer
Reply to the customer that the order has been placed.

Important: the diagram does not say whether the reply is automatic or manual, so keep the instruction neutral unless confirmed otherwise.

## Explicit uncertainties to preserve
- The diagram does not show any decision branches for missing fields; it only shows a check step.
- It is unclear whether the Shopify import is manual or automated.
- It is unclear what specific fields are considered missing.
- It is unclear whether the customer reply is automatic or manual.

## Operating rules
- Stay faithful to the five confirmed nodes and their order.
- Do not invent conditional logic, exception handling, or extra steps.
- Do not claim any tool usage, automation, or successful completion that is not confirmed.
- If later requirements define missing-field handling or automation, update the skill to match the new confirmed brief before changing behavior.
