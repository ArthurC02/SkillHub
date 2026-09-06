---
name: return-request-7day-reply
description: When a customer return request includes dates, determine whether it is within 7 days and generate the matching standard reply email. Use this skill for triaging return requests and producing the appropriate Chinese response template.
---

# Return Request 7-Day Reply

## Purpose
When a customer return request includes date information, determine whether it is within 7 days and produce the matching standard reply email in Chinese.

## Instructions
1. Read the user's input as the complete source of truth.
2. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
3. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
4. Identify the request date and purchase date from the input if they are given.
5. Compare the dates exactly as provided and determine whether the request is within 7 days of the purchase date.
6. If the input clearly shows the request is within 7 days, output the Chinese standard reply for the within-7-days case.
7. If the input clearly shows the request is over 7 days, output the Chinese standard reply for the over-7-days case.
8. If the input does not provide enough information to determine the result, say the missing information is not given.
9. Output the result in Chinese.
10. Include the final reply email text directly in the output so it can be used as-is.

## Output shape
- State whether it is within 7 days or over 7 days, or state that the result is not given.
- Then provide the standard reply email content.
- Do not ask follow-up questions when the input already contains enough information.

## Standard reply content
- Within 7 days: use a polite approval-oriented return reply that confirms the request can proceed according to policy.
- Over 7 days: use a polite rejection-oriented return reply that explains the request is outside the 7-day window according to policy.

## Constraints
- Use only the dates and customer text present in the input.
- Do not invent policy details beyond the 7-day condition.
- Do not include extra branches or conditions not supported by the input.