---
name: shopify-excel-order-import
description: Use this skill when you receive customer orders in Excel and need to prepare them for Shopify import, including converting to CSV, checking for missing fields, and returning the processed order file to the customer.
---

# Shopify Excel Order Import

Follow this flow when a customer sends an Excel order file for Shopify processing.

## 1) Receive the Excel order file
- Confirm you have the customer’s Excel order file.
- Identify the sheet that contains the order data.
- If there are multiple sheets, use the one that contains the actual order rows.

## 2) Convert the file to CSV format
- Save or export the order sheet as CSV.
- Preserve the original column names and row order unless a correction is required later.
- If the workbook contains formatting, formulas, or multiple tabs, export only the data needed for import.

## 3) Check for missing fields
- Review the CSV for required Shopify import fields and any customer-specific required columns.
- Look for blank cells, incomplete rows, inconsistent values, and obvious formatting problems.
- Common issues to check:
  - missing customer name or contact details
  - missing product identifiers or variants
  - missing quantities
  - missing shipping or address information if required
  - malformed dates, phone numbers, or email addresses
- If a required field is missing and cannot be inferred safely, flag it for correction rather than guessing.

## 4) Prepare for Shopify backend import
- Clean the CSV so it is ready for import into Shopify.
- Standardize values where needed so the file is consistent and importable.
- Ensure the final CSV matches the expected Shopify import structure used by the team or customer.
- If the data cannot be imported as-is, note the exact rows and fields that need attention.

## 5) Return the processed order to the customer
- Provide the completed CSV or the import-ready file back to the customer.
- Briefly state that the order file has been converted, checked, and prepared for Shopify import.
- If any missing fields were found, list them clearly so the customer can correct them.

## If the task cannot be completed from the provided file
- Ask the customer for the missing workbook, the correct sheet, or clarification on required fields.
- Do not invent missing order data.
- Do not claim the file has been imported unless you actually performed the import in Shopify.
