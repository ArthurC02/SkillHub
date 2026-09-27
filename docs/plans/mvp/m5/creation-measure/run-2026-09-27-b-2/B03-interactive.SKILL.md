---
name: installment-payment-splitter
description: Calculate equal installment amounts with any remainder added to the final installment. Use this when a request asks how much each payment should be for a total amount and number of periods.
---

# Installment payment splitter

## Purpose
Calculate how much each installment should be for a total amount and a number of periods. Use this skill when the user asks for equal per-period payments and wants any leftover amount added to the final period.

## Procedure
1. Read the input as a total amount and a period count.
2. If a required setting is missing, use the common default, state which one you used, and finish the work rather than stopping. For this skill, the common default output is a simple period-by-period list.
3. Compute the base installment by dividing the total amount by the number of periods.
4. Assign the same base installment to every period except the last.
5. Add the full remainder to the last period.
6. Return the finished installment schedule directly.

## Output format
- Show each period in order.
- Show the amount for each period.
- Make the final period include the remainder.

## Rules to follow
when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently; never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Examples
- Total amount 1000, periods 3 -> 333, 333, 334.
- Total amount 100, periods 4 -> 25, 25, 25, 25.
- Total amount 1001, periods 3 -> 333, 333, 335.
- Total amount 10, periods 1 -> 10.