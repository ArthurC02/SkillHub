---
name: typhoon-office-notice-writer
description: Write a formal internal typhoon office-closure notice when the user provides the effective date, resume-work date, and departments that must remain on duty.
---

# Typhoon Internal Notice Writer

## What this skill does
Write a formal internal announcement email about a typhoon-related office closure or class suspension when the user provides the decision details.

## When to use it
Use this skill when the request is to draft an internal notice that must clearly state:
- the effective date
- the resume-work date
- the departments that must remain on duty

## Instructions
1. Read the user’s input and identify the closure decision and the required dates and department names.
2. Write the finished announcement email directly in one pass.
3. Use a formal, clear, internal-company tone.
4. Include all of the following explicitly in the email:
   - the effective date
   - the resume-work date
   - the names of the departments that must remain on duty
5. If the user provides a company name, subject line style, or audience, use it. If not, choose a common default and finish the email instead of asking more questions.
6. If a required fact is missing, do not invent it. Mark only that missing fact as not given, in the same language as the output, and continue with the rest of the notice.
7. If the input combines requirements that cannot both be fully met, keep the hard limit and say in one line what was left out.
8. Do not add unsupported facts such as specific weather causes, government orders, exact times, or policy details unless the user provides them.
9. Do not ask follow-up questions at the end. Deliver the finished email content itself.
10. Keep the output ready to send as an internal notice; do not describe these instructions.

## Output shape
Produce only the email draft, with a clear subject line and body.

## Suggested default structure
- Subject line
- Greeting
- Announcement of the typhoon-related closure
- Effective date
- Resume-work date
- Departments that must remain on duty
- Closing and signature block if provided by the user; otherwise use a neutral internal-company sign-off