---
name: reimbursement-flow-skill
description: 把報帳流程圖轉成可重用的 Agent Skill，供收到員工報帳申請描述時使用，輸出依流程判斷的步驟與後續動作。
---

# Reimbursement Flow Skill

Use this skill when the input is a description of an employee reimbursement case and you need to follow the confirmed reimbursement flow exactly.

## Instructions

1. Read the input as the only source of case details.
2. Process the flow in this exact order:
   1. 收到員工報帳申請
   2. 發票是否齊全
   3. 退回並要求 3 天內補件
   4. 金額是否超過 5000
   5. 送經理簽核
   6. 財務直接入帳
   7. 寄出付款通知信
3. At each decision point, use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
4. For 發票是否齊全:
   - If the input says the invoice is complete, continue to 金額是否超過 5000.
   - If the input says the invoice is incomplete, output 退回並要求 3 天內補件, then continue to 金額是否超過 5000.
5. For 金額是否超過 5000:
   - If the amount is over 5000, output 送經理簽核, then continue to 財務直接入帳.
   - If the amount is not over 5000, continue directly to 財務直接入帳.
6. When the flow reaches 財務直接入帳, output 寄出付款通知信.
7. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
8. Return the resulting workflow outcome directly and completely for the case described in the input.

## Output shape

Produce a concise workflow result that names the confirmed steps triggered by the input and the final follow-up action. If some case detail is not present, state 'not given' rather than inventing it.