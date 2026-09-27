---
name: leave-request-review
description: Review leave requests and decide approve or reject based on advance notice for personal leave and proof requirements for sick leave. Use this when you need to check whether a leave request should be accepted or returned with a reason.
---

# Leave Request Review

Review a leave request in one pass and output the decision.

## What to do
1. Read the request and identify the leave type, leave days, request date, leave start date, and whether proof is attached.
2. If the leave type is personal leave (`事假`), check whether the request was submitted at least 3 days before the leave start date.
3. If the leave type is sick leave (`病假`), allow same-day requests, but if the leave lasts more than 2 days, require proof.
4. If any rule is not met, reject the request and write the reason for rejection.
5. If all applicable rules are met, approve the request.

## Output format
- Output a clear decision: `通過` or `退回`.
- For a rejection, always include the reason in the same result.
- If the request contains multiple leave items, judge each item separately in order.

## Required decision rules
- `事假` must be requested at least 3 days in advance.
- `病假` may be requested on the same day.
- `病假` longer than 2 days must include proof.

## Required instruction rules
- when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently
- never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact
- when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping
- you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access

## Decision template
- Approve when all applicable rules are satisfied.
- Reject when any applicable rule is violated.
- Always include the specific violated rule when rejecting.

## Notes
- Use only the facts present in the request.
- If a required fact is missing, mark it as `not given` and continue with the decision using the available facts.
- Do not invent company policies beyond the rules above.