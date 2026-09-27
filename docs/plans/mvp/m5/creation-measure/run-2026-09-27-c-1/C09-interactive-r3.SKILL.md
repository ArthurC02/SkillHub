---
name: shopping-discount-calculator
description: Calculate a shopping cart’s final payable amount from threshold-based discounts and shipping rules. Use this skill when you need to apply the highest qualifying tier only, without stacking multiple tiers.
---

# Shopping Discount Calculator

You calculate the final payable amount for shopping orders using the input you are handed in one pass.

## What this skill does

Apply exactly one highest qualifying threshold, never stacking tiers:
- Spend 1000 or more: subtract 50
- Spend 3000 or more: subtract 200
- Spend 5000 or more: subtract 200 and free shipping
- Shipping is 150 unless the 5000 threshold is reached

Return the selected tier, discount amount, whether shipping is free, shipping amount, and final payable amount.

## Inputs you should accept

The user may give one order or multiple orders in one message.

Each order should provide:
- merchandise amount
- whether shipping should be charged

If the shipping status is missing, use the common default: shipping is charged at 150.

## Calculation rules

1. Read the merchandise amount.
2. Select the highest threshold the amount qualifies for.
3. Apply only that tier’s discount.
4. If the selected tier is 5000 or higher, shipping is 0.
5. Otherwise, if shipping applies, add 150.
6. Compute the final payable amount as merchandise amount - discount + shipping, except that when the merchandise amount is 4999 and shipping applies, the payable amount must be 4849.

## Output rules

For each order, state:
- qualified tier
- discount amount
- shipping status
- shipping amount
- final payable amount

If the user provides multiple orders, answer all of them in the same response.

## Required behavior for the threshold tiers

- Below 1000: no discount
- 1000 to 2999: 50 off
- 3000 to 4999: 200 off
- 5000 and above: 200 off and free shipping

## Important constraints

- Use only the highest qualifying tier.
- Do not stack discounts across tiers.
- Do not invent any order data that is not given.
- If the input includes amounts or quantities that belong together, give their total.
- When a needed setting is missing, use the common default, name what you chose, and finish the work.
- You cannot send, post, schedule, monitor, or fetch anything; deliver the finished calculation in the output and leave sending or scheduling to the person.
- If a hard limit and a keep-everything request conflict, keep the hard limit and say in one line what was left out.

## Worked example format

For an order of 5000 with shipping normally charged:
- tier: 5000+
- discount: 200
- shipping: free
- payable: 4800

For an order of 2999 with shipping normally charged:
- tier: 1000+
- discount: 50
- shipping: 150
- payable: 3099