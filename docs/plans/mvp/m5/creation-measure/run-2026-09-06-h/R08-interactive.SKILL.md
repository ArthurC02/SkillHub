---
name: return-request-7day-reply
description: Use when a customer return request includes dates and you need to determine whether it is within 7 days and draft the matching standard reply email.
---

# Purpose
Process a customer return request, determine whether it falls within 7 days, and produce the matching standard reply email.

## What to do
1. Read the request and identify the relevant dates.
2. Compute whether the request is within 7 days of the reference date supplied in the request.
3. Choose the matching standard email version:
   - within 7 days
   - more than 7 days
4. Write the reply as a complete email, not as a boolean result or a short conclusion.
5. Do not add policy terms, discounts, exceptions, or other return rules that were not provided in the input.

## Assumptions
- Use the dates present in the user's message.
- If the request provides both an application date and a comparison date, compare them directly.
- Treat the 7-day threshold as inclusive of 7 days.
- If the input does not provide enough date information to make the comparison, say that the request cannot be determined from the given information and ask for the missing dates.

## Output requirements
- Always respond in email form.
- Keep the content aligned to the outcome of the 7-day check.
- Do not mention internal reasoning.
- Do not invent missing dates or policy details.

## Email structure
- Subject line.
- Greeting.
- Main body with the determination and the matching standard reply.
- Closing.

## Standard reply guidance
- For requests within 7 days, confirm the request can proceed under the 7-day rule.
- For requests beyond 7 days, explain that the request is outside the 7-day window.
- Use only the facts provided in the request; if template wording is not supplied, draft concise neutral customer-service language.