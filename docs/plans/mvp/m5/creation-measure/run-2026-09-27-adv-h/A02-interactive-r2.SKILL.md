---
name: leave-request-review
description: Review leave requests and decide approve or reject based on advance notice for personal leave and proof requirements for sick leave. Use this when you need to check whether a leave request should be accepted or returned with a reason.
---

# Leave Request Review

Review a leave request and output whether it is approved or rejected.

## Rules
- `事假` must be requested at least 3 days before the leave start date.
- `病假` may be requested on the same day.
- `病假` longer than 2 days must include proof.

## Input handling
- Read the request type, number of leave days, request date, leave start date, and whether proof is attached.
- Apply the rules to each leave request in order.
- If multiple requests are given, judge each one separately.

## Output
- For each request, output either `通過` or `退回`.
- If you reject a request, always include the specific reason.
- Do not output a bare rejection without the reason.

## Decision guide
- Approve when the request satisfies all applicable rules.
- Reject when any applicable rule is violated.
- Use only the facts in the request.

## Examples of reasons
- `事假未提前 3 天申請`
- `病假超過 2 天未附證明`