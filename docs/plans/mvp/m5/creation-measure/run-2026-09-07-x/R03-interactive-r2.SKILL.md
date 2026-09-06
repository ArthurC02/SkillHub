---
name: daily-news-title-summary-email
description: Use this skill when you need a daily morning workflow that collects headlines from three specified news sites, condenses them into 5 summaries, and prepares the result for email delivery to a provided address.
---

# Daily News Title Summary Email Skill

Use this skill when the user wants a daily morning digest built from three specified news websites, summarized into 5 items, and sent to an email address they provide.

## Instructions

1. Read the user's request and extract the three news site URLs, the recipient email address, and any stated language or formatting preference.
2. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
3. Deliver the finished artifact itself in the output — never a description of the rules, a plan, a request for access, or instructions for the user to perform any manual step.
4. If any required input is missing, stop and ask only for that missing input; otherwise proceed without asking the user to paste, copy, or click anything.
5. Fetch the headlines from each of the three provided news site URLs.
6. If a source page is unavailable or blocked, use the available input only and do not invent the missing headlines.
7. Produce exactly 5 concise summary items from the collected headlines.
8. Include the three source URLs and the recipient email address in the final output, and if email sending is part of the surrounding workflow, provide the email-ready content rather than telling the user to send it manually.
9. Do not introduce any additional news sources, destinations, assumptions, or user-operated follow-up steps.

## Output requirements

- Return a finished summary ready for email delivery.
- Keep the result aligned to the user's stated language and formatting preference when provided.
- If a requested detail is not given, state 'not given'.

## Acceptance behavior

- The output names the three provided news site URLs.
- The output uses the provided recipient email address.
- The output contains exactly 5 summary items.
- The output does not add any news source not present in the input.
- The output does not require manual steps beyond the tools used by the skill.