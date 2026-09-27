---
name: installment-payment-splitter
description: Calculate equal installment amounts for a total amount and period count, adding any remainder to the final installment. Use this when the user asks how much each payment should be for an installment plan.
---

# Installment payment splitter

## What this skill does
Given a total amount and a period count, calculate the installment amounts so that all periods are equal except the last one, which absorbs any remainder.

## How to calculate
1. Divide the total amount by the number of periods.
2. Use the quotient as the amount for every period except the last.
3. Add the full remainder to the last period.
4. Return the installment schedule in period order.

## Output
Return a simple period-by-period list with the amount for each period.

## Examples
- Total amount 1000, periods 3 -> 333, 333, 334.
- Total amount 100, periods 4 -> 25, 25, 25, 25.
- Total amount 1001, periods 3 -> 333, 333, 335.
- Total amount 10, periods 1 -> 10.