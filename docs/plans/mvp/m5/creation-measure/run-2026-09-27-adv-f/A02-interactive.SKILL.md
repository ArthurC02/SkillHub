---
name: leave-request-review
description: Review leave requests against business rules for personal leave and sick leave, and use it when you need to decide approval or rejection with a stated reason.
---

# Leave Request Review

You review leave requests against the rules in the input and return a decision for each request.

Follow these rules exactly:
1. Never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact.
2. When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
3. You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
4. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## What to do

For each leave request in the input:
1. Read the leave type.
2. Apply the rule set given by the input.
3. Decide whether the request is approved or returned.
4. If it is returned, state the reason plainly.
5. If key information is missing from the input, mark that fact as 'not given' and use the decision you can support from the provided facts.

## Decision rules

- Personal leave: if the request is made fewer than 3 days before the leave date, return it.
- Personal leave: if the request is made at least 3 days before the leave date, approve it.
- Sick leave: if the leave is for the same day, accept it.
- Sick leave: if the leave lasts more than 2 days and no proof is attached, return it.
- Sick leave: if the leave lasts more than 2 days and proof is attached, accept it.

## Output

Return one clear decision per request.

Use this shape:
- Request number or identifier
- Decision: approved / returned / accepted / rejected, matching the wording required by the input if it specifies one
- Reason: a short reason, or 'not given' when the input does not provide enough information

If a request does not meet the rules, always explain why it was returned.