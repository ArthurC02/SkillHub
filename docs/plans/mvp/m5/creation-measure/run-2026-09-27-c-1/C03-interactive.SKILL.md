---
name: extract-order-email-summary
description: Extract ordered items, quantities, unit prices, and delivery address from customer order emails. Use this when you need a structured list from pasted email text.
---

# Order Email Summary Extractor

Create a clean, structured summary from the email text the user provides.

## What to do
1. Read the email content exactly as given.
2. Extract every ordered item that is explicitly stated.
3. For each item, capture:
   - item name
   - quantity
   - unit price
4. Extract the customer-specified delivery or receiving address if it appears.
5. Keep items separate; do not merge different products into one line.
6. Preserve the original values from the email.
7. Do not guess missing quantities, prices, product names, or addresses.
8. If the email includes multiple items, list them all.

## Output format
Return a clear list with one entry per item, plus the address section if present.

Use this default structure:

- Order items:
  - Item 1: name, quantity, unit price
  - Item 2: name, quantity, unit price
- Delivery / receiving address:
  - Address
  - Recipient name if given

## Rules
- Only use information explicitly present in the email.
- If a required detail is missing, mark it as not given in the language of the output.
- Do not invent facts.
- Do not perform calculations or convert units unless the user explicitly asks for that.
- If the email contains both order details and delivery details, include both in the final result.
- Deliver the finished summary itself, not an explanation of how it was made.

## When to use this skill
Use this skill whenever the user pastes order email text and wants the ordered products, quantities, unit prices, and delivery address organized into a list.