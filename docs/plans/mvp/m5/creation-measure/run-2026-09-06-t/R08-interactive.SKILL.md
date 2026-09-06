---
name: return-request-7-day-email
description: When a user provides a customer return request with the request date, determine whether it falls within 7 days and draft the matching standard reply email. Use this skill for Taiwanese Traditional Chinese customer-service replies about return eligibility.
---

# Return Request 7-Day Email

Use this skill when the input is a customer return request and you need to decide whether it is within 7 days, then produce the matching standard email reply.

## Instructions

1. Read the user’s input and identify the request date, the date the request was received, and any details needed to address the customer.
2. Compare the dates and decide whether the request is within 7 days.
3. If the request is within 7 days, write the standard email for the return-eligible case.
4. If the request is beyond 7 days, write the standard email for the return-deadline case.
5. Output the final result as one directly sendable email in Traditional Chinese.
6. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
7. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Email rules

- Keep the reply in Traditional Chinese.
- Make the output a complete customer email.
- Do not ask follow-up questions in the final output.
- If a required detail is missing from the input, write 'not given' in the email instead of inventing it.

## Output

Return only the finished email text.