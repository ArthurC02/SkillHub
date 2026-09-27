---
name: installment-payment-trial
description: Calculate installment payment amounts from a total amount and number of periods, with any rounding or tail difference absorbed into the final installment. Use this when a user needs a per-period payment schedule and a total-check output.
---

# Installment payment trial

## What this skill does
Given a total amount and a number of periods, calculate the payment for each period so that the first periods are identical and any tail difference is added to the final period. Return the full per-period list and a check that the payments sum to the total amount.

## How to work
1. Read the user's total amount and period count from the input.
2. If a required setting is missing, use the common default, state which default you used, and finish the work instead of stopping.
   - For working-day length, use the common default of 8 hours.
   - For tone, use the common default of neutral.
   - For format, use the common default of a simple numbered list.
3. Compute the base installment as the total amount divided by the number of periods.
4. Make the first periods identical.
5. Put any remainder, rounding difference, or tail difference into the final installment.
6. Output each installment explicitly, then show the total sum check.
7. If the input gives more than one example, handle every example in the same response.

## Rules to follow
- When the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output shape
- Use the format requested by the input if one is given.
- Otherwise, use a simple numbered list.
- For each case, show:
  1. the total amount,
  2. the period count,
  3. the base installment,
  4. the per-period amounts,
  5. the total sum check.

## Example behavior
- If the total amount divides evenly by the number of periods, all periods are the same.
- If it does not divide evenly, the last period absorbs the tail difference.
- Keep the result concise and directly usable.