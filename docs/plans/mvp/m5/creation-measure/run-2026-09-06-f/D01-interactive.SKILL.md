---
name: employee-expense-claim-flow
description: 判定員工報帳申請的處理結果與下一步動作；當你要把報帳流程圖轉成可執行的文字判定規則時使用。
---

# Employee Expense Claim Flow

## Purpose
This skill turns one employee expense claim input into the process outcome and next action based only on the confirmed flow rules.

Use it when you need to decide whether a reimbursement claim should be returned for correction, sent for manager approval, directly posted by finance, or followed by a payment notification.

## Input
Treat the user’s message as one reimbursement request containing, at minimum:
- whether the invoice set is complete
- the reimbursement amount

If the input is incomplete, ask only for the missing fields needed to follow the flow.

## Decision flow
1. Read the claim as a single case.
2. Check whether the invoices are complete.
   - If invoices are not complete, output that the claim is returned and must be supplemented within 3 days.
   - If invoices are complete, continue to the amount check.
3. Check whether the amount is greater than 5000.
   - If the amount is greater than 5000, send it for manager approval.
   - If the amount is 5000 or less, skip manager approval.
4. After manager approval, finance posts the reimbursement directly.
5. After finance posts the reimbursement, send the payment notification email.

## Required output
Return a concise result with these parts in order:
- decision
- next action
- reason based on the flow

Keep the original decision order. Do not move the payment notification before finance posting.

## Output style
Use clear Chinese wording in the result, matching the process language from the input when possible.

## Rules
- Do not invent extra approval steps.
- Do not add policy beyond the confirmed flow.
- Do not assume alternative branches that are not present in the input.
- If the claim is returned for missing invoices, state the 3-day supplementation requirement explicitly.

## Example interpretation
For an input where invoices are incomplete and the amount is 6200, the claim is still returned first because invoice completeness is checked before the amount branch in the confirmed flow.