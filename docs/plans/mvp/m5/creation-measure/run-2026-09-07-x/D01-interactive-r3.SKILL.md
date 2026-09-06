---
name: employee-reimbursement-flow-summary
description: 將員工報帳流程圖或流程文字整理成結構化、可執行的流程說明；當你需要把報帳步驟、判斷條件、分支與未明示處整理成一致格式時使用。
---

# Employee reimbursement flow summary

Use this skill when the user provides an employee reimbursement process description or flowchart content and wants it turned into a clear, structured flow summary.

## Instructions

Follow the confirmed nodes in order and keep every step grounded in the input you were handed.

1. 收到員工報帳申請
   - Start from the reimbursement request as given.
   - State the request exactly as the input provides it.

2. 發票是否齊全
   - Treat this as a condition.
   - If the input gives a yes/no branch, include both branches.

3. 退回並要求 3 天內補件
   - Use this only when the input shows the invoice is not complete.
   - Keep the 3-day limit exactly as given.
   - If the input does not say what happens after補件, put that follow-up in `uncertainties` as `not given`.

4. 金額是否超過 5000
   - Treat this as a condition.
   - Preserve the 5000 threshold exactly as given.
   - Do not infer any post-condition loopback unless the input states it.

5. 送經理簽核
   - Use this only when the input shows the amount exceeds 5000.

6. 財務直接入帳
   - Use this only when the input shows the amount does not exceed 5000, or after manager approval if the input states that flow.

7. 寄出付款通知信
   - End with the payment notification only if the input shows that final step.

8. 未明示回圈/後續流向
   - If the input says to return for correction, resubmission, or another action but does not specify what happens next, do not continue the flow; list the missing follow-up under `uncertainties` as `not given`.

## Output shape

Return exactly four sections with these headings, in this order: `## nodes`, `## conditions`, `## branches`, `## uncertainties`.
- Each heading must appear exactly once.
- Do not add any other top-level headings before, between, or after them.
- `nodes` lists the flow steps.
- `conditions` lists the decision points.
- `branches` lists each yes/no or next-step path.
- `uncertainties` lists only what the input does not specify, including any missing loopback or post-step flow.
- `## uncertainties` must always appear as a separate section, even when the only item is `not given`.
- Never omit the uncertainties section.

List each confirmed node under `nodes`.
List each confirmed decision point under `conditions`.
For `branches`, write the yes/no or next-step flow exactly as the input shows it.
For `uncertainties`, record only what the input does not specify. Always include at least one uncertainty item when the input leaves any loopback, retry, or post-step flow unspecified, even if the item is "not given".

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Notes

- Do not invent missing loopbacks, exception handling, or alternate approvals.
- Do not ask follow-up questions unless the input itself is missing the information needed to produce the summary.
- Keep the response in Traditional Chinese if the user input is in Traditional Chinese.
- Keep numbers, deadlines, and labels exactly as provided.