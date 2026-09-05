---
name: excel-order-to-shopify-csv
description: Convert customer Excel order data into CSV, check required fields for missing values, prepare Shopify import data, and generate a customer-ready "already submitted" reply when you receive an order spreadsheet that needs processing.
---

# Excel order to Shopify CSV

## Purpose
Use this skill when you receive a customer Excel order that must be turned into CSV, checked for missing required fields, prepared for Shopify backend import, and summarized with a reply message to the customer.

## Procedure
1. Read the incoming Excel order data and identify each row and column as provided.
2. Determine the required fields from the order content actually present in the file. Do not invent fields that are not supported by the input.
3. Check whether any required field is missing or blank.
4. Convert the order data into CSV using the same values from the Excel input.
5. Prepare Shopify import content from the CSV-ready data.
6. If the required fields are complete, generate a short customer reply stating that the order has been received and entered.
7. If required fields are missing, report the missing fields clearly and do not fabricate values.

## Output format
Return the result in three clearly separated sections:

### 1) CSV
Provide the CSV representation of the order data.

### 2) Missing field check
List missing required fields, or state that none are missing.

### 3) Customer reply
Provide a short, ready-to-send message confirming the order has been entered when the data is complete.

## Rules
- Use only the values present in the Excel order data.
- Do not add fallback data, assumptions, or extra workflow steps that are not present in the input.
- Do not claim the Shopify import succeeded unless the provided input explicitly shows that it succeeded.
- Keep the output structured so the three sections are easy to extract.
- If the input is incomplete, still return the CSV and missing field check, but make the customer reply reflect that the order cannot yet be completed.