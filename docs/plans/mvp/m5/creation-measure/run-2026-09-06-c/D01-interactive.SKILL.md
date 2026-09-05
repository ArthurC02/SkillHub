---
name: employee-expense-reimbursement
description: 處理員工報帳申請流程；當你需要依發票是否齊全與報帳金額是否超過 5000 來決定退回、送經理簽核或直接入帳時使用。
---

# Employee Expense Reimbursement

## Purpose
Use this skill to handle an employee expense reimbursement request with two checks: whether receipts/invoices are complete, and whether the amount exceeds 5000.

## Inputs
- Employee reimbursement application data
- Receipt/invoice completeness status
- Reimbursement amount

## Output
- A routing decision for the reimbursement request
- A status message indicating one of these outcomes:
  - Return for supplementation within 3 days
  - Send to manager for approval
  - Book directly by finance
  - Send payment notification email

## Procedure
1. Receive the employee reimbursement application.
2. Check whether the invoices are complete.
3. If the invoices are not complete:
   - Return the request.
   - Ask the employee to supplement the missing documents within 3 days.
   - Stop.
4. If the invoices are complete:
   - Check whether the reimbursement amount is greater than 5000.
5. If the amount is greater than 5000:
   - Send the request to the manager for approval.
   - After manager approval, hand the request to finance for booking.
6. If the amount is 5000 or less:
   - Finance books the reimbursement directly.
7. After finance booking:
   - Send a payment notification email.

## Rules
- Do not invent additional review steps, roles, or exception branches.
- Follow the invoice-completeness check before any later routing.
- Use the 5000 threshold exactly as the decision boundary.
- Keep the flow limited to the confirmed branches only.

## Notes
- The flow assumes that once invoices are complete, the amount check is applied next.
- The diagram does not explicitly state whether a supplemented request must re-enter the process, so this skill should not add that behavior unless the workflow owner confirms it.