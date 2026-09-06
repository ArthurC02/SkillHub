---
name: employee-reimbursement-flow
description: Use this skill when you need to turn the confirmed employee reimbursement flowchart into a reusable skill that processes reimbursement requests by following the chart’s steps and branches.
---

# Employee Reimbursement Flow

Use this skill when the input is a confirmed employee reimbursement flowchart and you need a reusable skill that follows that flow exactly.

## Instructions

- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- Follow the flowchart in node order.
- If any required detail is missing from the input, write 'not given' for that detail rather than inventing it.

## Steps

1. **收到員工報帳申請**
   - Start from the reimbursement request provided in the input.
   - If the input does not give the request details, write 'not given'.

2. **發票是否齊全**
   - Check whether the invoices are complete.
   - If this is silent in the input, write 'not given'.

3. **退回並要求 3 天內補件**
   - If the invoices are not complete, return the request and require补件 within 3 days.
   - After that, continue to the amount check.

4. **金額是否超過 5000**
   - Check whether the amount exceeds 5000.
   - If this is silent in the input, write 'not given'.

5. **送經理簽核**
   - If the amount is over 5000, send it for manager approval.

6. **財務直接入帳**
   - If the amount is not over 5000, finance posts it directly.
   - If the amount is over 5000, finance posts it after manager approval.

7. **寄出付款通知信**
   - After finance posts the reimbursement, send the payment notification email.

## Output

Return the finished reimbursement processing artifact itself, following the confirmed flow exactly and adding nothing beyond the confirmed nodes, conditions, and branches.

If the input is silent on any detail, write 'not given'.