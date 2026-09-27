---
name: leave-application-review
description: 審核請假申請，依事假提前 3 天、病假超過 2 天需附證明的規則判斷是否通過；當需要退回時，用來輸出明確的退回原因。
---

# Leave Application Review

You are given one leave-application request. Decide whether it passes the stated rules and return the finished result itself.

## Instructions

- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- Read the request exactly as provided.
- Check the leave type, application date, leave start date, leave duration, and whether proof is attached when they are present.
- Apply only the rules stated in the input.
- If the input is missing information needed to apply a rule, write 'not given' for that missing item and state that the decision cannot be completed from the provided input.
- If the request does not meet a rule, return the rejection reason clearly.

## Decision rules

### 1. Personal leave

If the leave type is personal leave, compare the application date with the leave start date.

- If the application date is fewer than 3 days before the leave start date, reject the request and explain that personal leave must be applied for at least 3 days in advance.
- If the application date is at least 3 days before the leave start date, approve the request.

### 2. Sick leave

If the leave type is sick leave:

- If the leave duration is 2 days or fewer, approve the request.
- If the leave duration is more than 2 days and proof is attached, approve the request.
- If the leave duration is more than 2 days and proof is not attached, reject the request and explain that sick leave over 2 days requires proof.

## Output

Return the result directly in a concise format that includes:

- the decision: approve or reject
- the reason when the request is rejected

If any required value is not given, keep the missing value as 'not given' and say the decision cannot be completed from the provided input.