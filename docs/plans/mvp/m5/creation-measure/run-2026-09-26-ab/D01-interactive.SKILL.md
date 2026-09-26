---
name: employee-expense-flow-summary
description: 將已確認的員工報帳流程圖整理成忠實的流程說明；在需要把流程圖轉成可直接使用的文字時使用，且不得加入圖上未出現的資訊。
---

# Employee Expense Flow Summary

Use this skill when the input is an employee reimbursement flowchart or a confirmed diagram description that needs to be turned into a faithful process explanation.

Follow the confirmed diagram exactly, in order, and do not add any date, name, assumption, step, or branch that the input does not give.

## Instructions

1. Read the input and identify the confirmed flow steps, conditions, and branches.
2. Walk the diagram nodes in order, from the first node to the last node.
3. Describe each node using only the words supported by the input.
4. Treat the confirmed conditions as decision points and preserve their branches.
5. If a detail is silent in the input, write `not given` instead of inventing it.
6. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
7. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
8. If the input is missing the flow content needed to complete the explanation, refuse briefly and state what is missing.

## Output requirements

- Output a single, direct process explanation.
- Keep the order of the confirmed nodes.
- Keep the confirmed branching logic.
- Do not add extra approval roles, currencies, deadlines, or alternate paths unless they appear in the input.
- If the input includes a confirmed final action, present it as the last step.

## Checked flow to preserve

- 收到員工報帳申請
- 檢查發票是否齊全
- 發票不齊全時：退回申請並要求3天內補件
- 發票齊全時：繼續判斷金額
- 判斷金額是否超過5000
- 金額超過5000時：送經理簽核
- 金額不超過5000時：由財務入帳
- 入帳後：寄出付款通知信