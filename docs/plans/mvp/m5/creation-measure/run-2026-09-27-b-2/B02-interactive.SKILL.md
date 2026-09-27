---
name: travel-expense-review
description: Review travel expense claims and decide whether they pass based on amount, receipt, and manager approval. Use this when you need to approve or reject reimbursement requests and return a clear rejection reason.
---

# Travel Expense Review

## Purpose
Review each travel expense claim in one pass and decide whether it passes or is returned with a reason.

## Rules to apply
- If the amount is less than 2000, manager approval is not required, but a receipt must be attached.
- If the amount is 2000 or more, manager approval is required, and a receipt must be attached.
- Any claim that does not pass must be returned with the reason.

## Output
For each claim, output either:
- `通過`, or
- `退回：` followed by the reason or reasons.

## Decision steps
1. Read the claim amount.
2. Check whether a receipt is attached.
3. Check whether manager approval is attached.
4. If the amount is below 2000:
   - pass only when a receipt is attached.
   - manager approval does not matter.
5. If the amount is 2000 or above:
   - pass only when both a receipt and manager approval are attached.
6. If the claim fails, state every applicable reason.

## Reason wording
Use plain reasons such as:
- `缺少發票`
- `缺少主管簽核`

## Do this in one pass
Process the input you are given directly. Do not ask for more information if the amount, receipt status, and approval status are present.

## Hard limits and missing information
- When the input makes two requirements impossible to meet together, keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give — no name, date, figure or event — and write `not given` only for such a missing fact.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
