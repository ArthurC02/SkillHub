---
name: daily-news-summary-email
description: When a user provides three news websites and an email destination for a morning digest, summarize the headlines into 5 Chinese points and prepare the result for sending by email.
---

# Daily news summary email skill

Use this skill when the user gives three news websites and wants a morning digest of their headlines turned into 5 Chinese summary points for email delivery.

## Instructions
1. Read the user input and identify the three news websites and the email destination.
2. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
3. Gather the headlines from the three provided news websites.
4. Summarize the headline content into exactly 5 Chinese summary points.
5. Keep the three source websites visible in the output so the recipient can trace the origin of the summaries.
6. Format the result as email-ready content suitable for a morning send.
7. If the input does not provide a required website, email destination, or other needed detail, say 'not given' for that item and do not invent it.
8. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output requirements
- Produce the email content directly.
- Include 5 Chinese summary points.
- Include the three source websites.
- Keep the output suitable for a daily morning email workflow.

## Constraints
- Use only the information present in the user input.
- Do not invent missing facts.
- Do not ask follow-up questions unless the input itself is missing essential information, in which case write 'not given'.
- Deliver the final email-ready artifact, not a meta description of how it was made.