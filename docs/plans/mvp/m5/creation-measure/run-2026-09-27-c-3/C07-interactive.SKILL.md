---
name: travel-expense-reconciliation
description: 核對員工出差報帳明細，逐日加總花費並比對是否與員工填寫的總計一致；當你收到出差報帳內容時使用。
---

# Travel Expense Reconciliation

Use this skill when you receive a travel reimbursement statement and need to verify the arithmetic.

## What to do

1. Read the full reimbursement details from the user input.
2. Identify each date or day block.
3. For each day, add all listed expenses for that day.
4. Add the daily totals to get the overall total.
5. Find the employee-entered grand total in the input.
6. Compare the computed overall total with the employee-entered grand total.
7. Report whether they match.
8. If they do not match, report the difference as computed total minus employee total, and also state the absolute gap.

## Output

Return a concise reconciliation result that includes:

- each day's total
- the overall total
- the employee-entered total
- whether they match
- the difference when they do not match

## Rules

- Do not invent any missing expense item, date, or total.
- If a required fact is not given, say it is not given in the same language as the input.
- Use the numbers exactly as provided in the input.
- If the input includes multiple amounts that belong together, give their total.
- If the user asks for more detail than the available input supports, keep the result to what can be verified from the given numbers.
- Deliver the finished reconciliation in the output; do not ask the user to do the calculation.
- Sending, posting, scheduling, monitoring, or fetching is left to the person using the skill.

## Assumptions

- Treat each expense amount listed under a date as part of that day's total.
- Treat the employee-entered grand total as the comparison target.
- Use the common default of simple arithmetic addition when no special accounting rule is stated.

## Verification focus

The result must be directly checkable from the input:

- the daily totals must be present when daily entries are present
- the overall total must be present
- the comparison result must be explicit
- any mismatch must include a numeric difference

## Output style

Be direct and numerical. Prefer a short tabular or bullet summary if it makes the comparison clearer.