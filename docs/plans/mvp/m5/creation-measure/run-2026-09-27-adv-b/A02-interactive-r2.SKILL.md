---
name: leave-request-review
description: 根據事假與病假的申請時點、天數與證明條件審核請假申請；當你需要自動判定請假是否通過、退回及退回原因時使用。
---

# leave-request-review

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Purpose

Review a leave request and decide whether to approve or reject it based only on the rules present in the input.

## Inputs

Read the request exactly as given. Use only the leave type, leave dates or duration, application date, and whether proof is attached if those details are present.

If any required detail for applying the rules is missing, write `not given` for that detail and report that the review cannot be completed from the provided input alone.

## Decision rules

1. If the leave type is `事假`, check whether the application date is at least 3 days before the leave start date.
   - If yes, approve.
   - If no, reject and state that 事假 must be applied for 3 days in advance.

2. If the leave type is `病假`, allow same-day application.
   - If the request is for more than 2 days, proof is required.
   - If proof is attached, approve.
   - If proof is missing, reject and state that proof is required.

3. If the request does not satisfy the applicable rule, reject it and clearly explain the reason for rejection.

4. Do not invent extra policy, exceptions, or thresholds.

## Output

Return the finished review itself in a concise, decision-oriented format.

Include:
- `result`: `通過` or `退回`
- `reason`: a direct explanation tied to the rule that applied
- any required missing detail written as `not given`

## Procedure

1. Identify the leave type.
2. Identify the leave start date, end date or duration, and application date if present.
3. For `事假`, compare the application date with the leave start date.
4. For `病假`, check whether the request is same-day and whether the duration is more than 2 days.
5. Check whether proof is attached when required.
6. Produce the final decision and reason.

## Required behavior

- Use only the input content.
- Never add assumptions.
- Never ask the user a question if the decision can be made from the input.
- If the input is missing information needed to apply a rule, say so explicitly with `not given`.
- If the request is non-compliant, write the rejection reason clearly.
- If the request is compliant, state approval clearly.
