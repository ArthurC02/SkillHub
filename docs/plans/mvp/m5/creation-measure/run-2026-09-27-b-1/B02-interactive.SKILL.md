---
name: travel-expense-reimbursement-review
description: Review business-trip reimbursement requests and decide approve or return based on amount, invoice attachment, and supervisor approval. Use this when you need a simple policy check that must also explain any rejection reason.
---

# Travel Expense Reimbursement Review

Review each reimbursement request against the provided rules and return a decision with a reason when the request fails.

## Rules to apply

1. If the amount is less than 2000, approve the request when an invoice is attached.
2. If the amount is 2000 or more, approve the request only when both an invoice is attached and supervisor approval is present.
3. If a request does not meet the rule for its amount range, return it and state the reason clearly.
4. Process the input you are given in one pass and produce the finished decision(s) directly.

## Required output

For each reimbursement request, output:
- the decision: `approve` or `return`
- a reason for every returned request

## Assumptions and defaults

- Treat the currency as TWD unless the input says otherwise.
- Treat `less than 2000` as `< 2000`.
- Treat `2000 or more` as `>= 2000`.
- If the input is missing a needed field, use `not given` for that missing fact and return the request with a reason that names the missing field.

## Instructions for the agent

- Never invent a fact the input does not give: no name, date, figure or event. Use `not given` only for such a missing fact.
- When the input makes two requirements impossible to meet together, keep the hard limit and say in one line what you left out — never drop it silently.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output shape

Return a compact decision list, one item per reimbursement request, with the decision and reason where needed.