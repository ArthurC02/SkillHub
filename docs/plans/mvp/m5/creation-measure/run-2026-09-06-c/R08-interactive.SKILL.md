---
name: return-request-7day-response
description: Use when a customer submits a return request and you need to check whether it falls within 7 days and reply with the appropriate standard email.
---

# Return Request 7-Day Response

## Purpose
Use this skill when a customer submits a return request and you must determine whether the request is within 7 days, then provide the matching standard reply email.

## What to do
1. Identify the request date and the reference date used for the 7-day check.
2. Calculate the day difference.
3. Classify the request as one of the following:
   - **Within 7 days**: the difference is 7 days or fewer.
   - **Over 7 days**: the difference is greater than 7 days.
   - **Cannot determine**: required date information is missing or ambiguous.
4. Return both:
   - the decision label, and
   - the standard email text that matches that decision.

## Rules
- Base the decision only on the 7-day timing rule.
- Do not invent or assume extra return-policy details.
- If the dates are incomplete, contradictory, or unclear, do not guess; state that the 7-day check cannot be determined.
- The output must include ready-to-use standard email text, not only a judgment.
- Keep the response aligned to the provided dates and the 7-day rule.

## Response structure
Provide the output in this order:
1. Decision label
2. Short reason with the date comparison
3. Standard email body

## Standard email content
### Within 7 days
- State that the return request is within 7 days.
- Confirm the request will be processed according to the standard return procedure.
- Use a professional and polite tone.

### Over 7 days
- State that the return request is outside the 7-day window.
- Explain that the request does not qualify under the standard 7-day rule.
- Use a professional and polite tone.

### Cannot determine
- State that the 7-day check cannot be completed because the required date information is missing or unclear.
- Ask the requester to provide the missing date information.
- Use a professional and polite tone.

## Output quality requirements
- Make the decision explicit.
- Ensure the email text can be sent directly after minimal editing.
- Do not include unsupported policy claims, compensation promises, or exceptions.
- If the input provides dates in different formats, normalize them before comparing them.