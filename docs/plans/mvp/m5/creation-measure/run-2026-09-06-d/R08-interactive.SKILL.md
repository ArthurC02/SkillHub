---
name: return-request-7-day-reply
description: Use when a customer return request arrives and you need to check whether it is within 7 days and reply with the matching standard email.
---

# Purpose

Use this skill when a customer submits a return request and you need to determine whether the request is within 7 days, then produce the matching standard reply.

# Inputs

- Return request date
- Reference date used for the 7-day check
- The two approved standard email templates:
  - one for requests within 7 days
  - one for requests beyond 7 days

If any of these are missing, do not guess. Ask for the missing information.

# Procedure

1. Read the return request date and the reference date.
2. Compute the number of calendar days between the two dates.
3. If the difference is 7 days or fewer, treat the request as within 7 days.
4. If the difference is greater than 7 days, treat the request as beyond 7 days.
5. Select the matching approved standard email template.
6. Return only the selected email, unless information is missing.
7. If information is missing, reply with a short list of the missing fields needed to decide.

# Output rules

- Keep the reply in email format.
- Do not invent policy details, compensation, tone rules, or extra promises.
- Do not rewrite the approved template unless the user explicitly supplies a version to use.
- Make the decision explicit in the reply when the required dates are available.
- If the request is within 7 days, use the within-7-days template.
- If the request is beyond 7 days, use the beyond-7-days template.

# Quality check

Before responding, verify that:

- the date comparison was performed using the supplied dates
- the branch chosen matches the day difference
- no missing field was silently assumed
- the final response stays faithful to the approved template