---
name: reimburse-flow-skill
description: Use this skill when you need to turn the confirmed reimbursement flowchart into a reusable Agent Skill that describes the process, conditions, branches, and any remaining uncertainty in clear Markdown without adding unconfirmed steps.
---

# Purpose
Turn the confirmed reimbursement flow understanding into a reusable Agent Skill description. Use this skill when the task is to restate the confirmed flowchart as a clean Markdown skill, and when the input is the confirmed diagram understanding or an equivalent confirmed summary.

# What to do
1. Read the confirmed flow in the order of its nodes.
2. Describe each confirmed node as a step, without adding any step that is not in the confirmed understanding.
3. Describe the confirmed conditions and branches exactly as confirmed.
4. Preserve the confirmed uncertainty as an explicit open point.
5. Keep the output reusable: write it as a skill body that another agent can follow, not as a one-off explanation.

# Confirmed flow
## 1. 收到員工報帳申請
Start with the reimbursement request being received.

## 2. 發票是否齊全
Check whether the invoice is complete.
- If the invoice is not complete, the flow goes to **退回並要求 3 天內補件**.
- If the invoice is complete, the flow goes on to the next decision.

## 3. 金額是否超過 5000
Check whether the amount exceeds 5000.
- If the amount exceeds 5000, the flow goes to **送經理簽核**.
- If the amount does not exceed 5000, the flow goes to **財務直接入帳**.

## 4. 退回並要求 3 天內補件
Return the request and require補件 within 3 days.

## 5. 送經理簽核
Send the case to the manager for approval.

## 6. 財務直接入帳
Finance posts the entry directly.

## 7. 寄出付款通知信
Send the payment notification email.

# Uncertainty to preserve
There is one remaining uncertainty in the confirmed diagram understanding: a loop on the left side from the invoice-complete decision to the amount-exceeds-5000 decision. Preserve this as an open question and do not resolve it unless the source diagram is rechecked.

# Output style
- Use Markdown.
- Keep the confirmed nodes, conditions, branches, and uncertainty intact.
- Do not invent additional approvals, exceptions, deadlines, or notifications.
- If asked to generate a skill from this understanding, produce the same structure in the same order.