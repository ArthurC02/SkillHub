---
name: leave-request-review
description: Review leave requests and decide pass or reject based on advance-notice and proof rules; use this when a user needs a request checked against these leave-policy constraints. If a request does not comply, return the rejection reason(s).
---

# Leave request review

Review the leave request using only the rules provided in the input. Decide whether the request passes or must be returned, and include the reason for any return.

## Rules to apply

- **Vacation leave** must be requested at least **3 days in advance**.
- **Sick leave** may be requested **on the same day**.
- If **sick leave exceeds 2 days**, it must include **proof**.
- If the request does not meet the rules, **return it and state the reason**.

## How to process the input

1. Read each leave request in the user input.
2. Identify the leave type, request date, leave start date, leave duration, and whether proof is provided.
3. Check the request against the rules above.
4. Produce a clear result for each request:
   - **Pass** when the request satisfies the rules.
   - **Return** when it does not, with the specific reason.

## Output requirements

- Give the result for each request in the same order as the input.
- State the decision plainly: **Pass** or **Return**.
- If a request is returned, say exactly which rule it breaks.
- Do not invent missing facts. If a required fact is not given, write **not given** for that fact and continue using the provided information.
- If the input mixes a hard limit with a request to keep everything, keep the hard limit and say in one line what you left out — never drop it silently.
- When a setting the work needs is missing, use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so if the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Response style

Keep the answer concise and operational. When multiple requests are present, list each one and its result. Include reasons only when a request is returned.