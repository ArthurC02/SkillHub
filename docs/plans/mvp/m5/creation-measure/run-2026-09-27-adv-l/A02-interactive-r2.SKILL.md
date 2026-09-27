---
name: leave-request-review
description: Review leave requests against specified policy rules and return approve/reject decisions with reasons. Use this when you need to check personal leave or sick leave applications for timing and proof requirements.
---

# Leave request review

Review each leave request against the rules provided in the input, then return a decision for every request.

## Required inputs
Use the fields the user gives you. If a needed setting is missing, use the common default, say which one you used, and finish the work.

Typical fields you may need:
- leave type
- application date
- leave start date
- leave length in days
- proof attached for sick leave

If the input does not give a fact you need, write `not given` only for that missing fact.

## Steps
1. Read the leave request(s) in the input.
2. Identify the leave type for each request.
3. Check the request against the rules stated in the input.
4. Decide `approve` or `reject` for each request.
5. If a request is rejected, state the exact rule it breaks and the reason.
6. Return the finished review for every request in one pass.

## Decision rules
- Personal leave: require application at least 3 days before the leave start date.
- Sick leave: may be applied for on the same day as the leave starts.
- Sick leave longer than 2 days: proof is required.
- Any request that does not meet the rules must be rejected with a clear reason.

## Output format
Return one result per request, in the same order as the input.

Use this shape:
- Request 1: approve/reject
- Reason: ...
- Request 2: approve/reject
- Reason: ...

If a request is approved, say why it passed the rule check.
If a request is rejected, name the violated condition explicitly.

## Operating rules
- When the input makes two requirements impossible to meet together (a length limit and `keep everything`), keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give — no name, date, figure or event — and write `not given` only for such a missing fact.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.