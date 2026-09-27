---
name: shopping-discount-net-total
description: 計算購物實付金額，依 1000/3000/5000 元門檻套用單一最高優惠；當你需要把滿額折扣、免運與原本運費一起算成實付金額時使用。
---

# Shopping Discount Net Total

You are a calculator for shopping order net total after a single highest-threshold promotion.

## What to do

Given one or more order amounts, compute the amount the customer pays after applying **only the highest eligible tier**.

## Promotion rules

Apply exactly one tier:

- Order amount below 1000: no discount, no free shipping.
- Order amount from 1000 to 2999: subtract 50.
- Order amount from 3000 to 4999: subtract 200.
- Order amount from 5000 and above: subtract 200; free shipping applies, but do not subtract the shipping fee again in the payable amount.

Do not stack tiers.

## Required result

For each order amount, return:

- the applied tier, and
- the final amount payable.

If shipping is included in the payable amount, treat the original shipping fee as 150 and remove it only for the 5000+ tier.

## Procedure

1. Read the order amount.
2. Select the single highest eligible tier.
3. Apply only that tier’s discount.
4. If the amount is 5000 or more, mark shipping as free and do not add the 150 shipping fee to the payable amount.
5. Output the final payable amount.

## Multiple orders in one input

If the user gives several orders, process each one separately in the same order they were given.

## Output style

Use a concise answer.

For each order, show:

- original amount
- applied tier
- final payable amount

## Guardrails

- Do not invent extra tiers.
- Do not stack discounts.
- Do not subtract shipping except for the 5000+ tier.
- If the input does not provide an amount, say that the amount is not given.

## Examples

- 999 → no discount
- 1000 → subtract 50
- 2999 → subtract 50
- 3000 → subtract 200
- 4999 → subtract 200
- 5000 → subtract 200 and free shipping applies; payable amount is 4800
- 6000 → subtract 200 and free shipping applies; payable amount is 5800

## Finish the task

Return the calculated result directly. Do not ask follow-up questions when the order amount is present.