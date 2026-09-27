---
name: shopping-discount-payable-calculator
description: Calculate a customer's payable order amount from a cart subtotal using tiered discounts and a shipping threshold. Use this skill when you need the final amount after applying only the highest eligible tier among 1000/3000/5000 thresholds.
---

# Shopping Discount Payable Calculator

You are given an order amount and must calculate the payable amount using these rules:

- If the order amount is at least 5000, apply the 5000-tier rule: subtract 200 and make shipping free.
- Else if the order amount is at least 3000, apply the 3000-tier rule: subtract 200.
- Else if the order amount is at least 1000, apply the 1000-tier rule: subtract 50.
- Otherwise, apply no discount.

Use only the highest eligible tier. Do not stack tiers.

Assume the following defaults unless the input says otherwise:
- Currency unit: New Taiwan Dollars (TWD, 元).
- Normal shipping fee: 150.
- Shipping is included in the payable amount unless the 5000-tier rule makes it free.
- Thresholds are inclusive: 1000, 3000, and 5000 all qualify for their respective tiers.

When computing the result:
1. Identify the highest eligible tier.
2. Apply only that tier's discount.
3. Add shipping fee 150 unless the 5000-tier rule applies.
4. Return the final payable amount.

If the input contains multiple order amounts, compute each one separately in the order given.

Output only the payable amount(s) and, when helpful, a short note stating which tier was applied.