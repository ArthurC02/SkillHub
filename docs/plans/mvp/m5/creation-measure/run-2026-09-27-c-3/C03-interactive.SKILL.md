---
name: order-email-to-list
description: Extract ordered items, quantities, unit prices, and the customer-specified shipping address from order emails, and use it when you need a structured checklist from customer order email text.
---

# Order Email to List

Extract structured order details from customer-provided email text and turn them into a clean list.

Use this Skill when you need to read one or more order emails and extract the ordered items, quantities, unit prices, and the customer-specified shipping address.

## Instructions

1. Read the email text provided by the user.
2. Identify every ordered item mentioned in the email.
3. For each item, extract:
   - item name
   - quantity
   - unit price
4. Extract the customer-specified shipping address exactly as written when it is present.
5. If there are multiple items, list all of them.
6. Do not add any item, quantity, price, or address that is not given in the email.
7. If the email contains more than one address-like string, use the one explicitly indicated as the shipping or delivery address.
8. If a requested field is not given, write that it is not given.
9. Output the result as a clear list.

## Output format

Use this structure:

- Customer name: ... if given; otherwise write `not given`
- Shipping address: ... if given; otherwise write `not given`
- Items:
  - Item 1: name, quantity, unit price
  - Item 2: name, quantity, unit price
  - Continue for every item found

## Rules

- Never invent missing facts.
- Keep all items found in the email.
- Preserve the email's wording for address and prices unless a direct normalization is needed for readability.
- If the input contains several order emails, process each one separately unless the user asks for a merged list.

## Result

Return the finished list directly in your response. Do not describe the process or ask follow-up questions.