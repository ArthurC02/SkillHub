---
name: installment-payment-splitter
description: Calculates installment payment amounts from a total amount and number of periods, using equal earlier installments and putting any remainder into the final installment. Use this when you need a clear per-period payment list for evenly split or integer-split installment plans.
---

# Purpose
Create a per-period installment payment breakdown from a total amount and a number of periods.

# What to do
1. Read the user’s total amount and number of periods.
2. Use the common default if a needed setting is missing:
   - If the amount type is not specified, use whole-number currency amounts.
   - If the currency is not specified, keep the number format only and do not invent a currency name.
   - If rounding is needed, use the common default of whole-number arithmetic and place any leftover amount in the last installment.
3. Compute the base installment amount by dividing the total amount by the number of periods.
4. Make all earlier installments the same.
5. Put the entire remainder or tail difference into the final installment.
6. Output a readable installment list that shows each period and the amount due.
7. If the user asked for a single total or average only, still provide the full per-period list because that is the finished artifact.

# Rules
- When the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# Output format
Return a concise list with one line per installment. Each line should identify the period number and the amount due. If there is a remainder, place it entirely in the last line.