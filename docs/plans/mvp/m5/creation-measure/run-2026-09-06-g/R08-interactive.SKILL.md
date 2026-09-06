---
name: return-request-7day-reply
description: Use when you receive a customer return request and need to decide whether it falls within 7 days, then draft the appropriate standard reply email.
---

# Return request 7-day reply

When given a customer return request, determine whether the request is within 7 days based only on the dates provided in the input, then draft the matching standard reply email.

## Procedure

1. Read the request and identify the relevant dates:
   - the date the customer received the item, or the date the return window starts
   - the current date, request date, or other date needed to judge the elapsed time
2. Compute whether the request is within 7 calendar days.
3. Choose one of two responses:
   - within 7 days: reply with the approved return-accepted standard email
   - over 7 days: reply with the approved return-rejected standard email
4. Write the response as a complete customer email.

## Output requirements

- State the decision clearly in the email.
- Keep the reply focused on the 7-day rule.
- Do not invent additional policy details.
- Do not ask follow-up questions if the necessary dates are present; make the decision directly.
- If the input does not contain enough date information to judge the 7-day window, say what date information is missing and stop.

## Style

- Use a polite, professional customer-service tone.
- Produce a ready-to-send email rather than notes or analysis.
- Include a subject line if the surrounding system expects email format; otherwise write the body of the email clearly and completely.

## Reference handling

- If an approved company template is provided in the task context, follow it.
- If no template is provided, generate a concise standard reply that clearly matches the decision.
- Do not copy unrelated wording from other templates whose purpose is different from return handling.

## Quality check

Before finishing, confirm that the reply:

- matches the 7-day decision
- is a complete email
- does not introduce unrelated policies
- uses only the information present in the request
