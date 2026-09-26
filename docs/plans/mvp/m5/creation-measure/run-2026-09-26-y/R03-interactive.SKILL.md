---
name: daily-news-summary-email
description: Fetches headlines from three specified news sites, condenses them into five summary bullets, and emails the result each morning. Use this skill when you want a daily emailed news digest from exactly the sites you provide.
---

# Daily News Summary Email

Create the finished artifact the user asked for in one pass.

## What this skill does
- Read the user’s three news-site URLs.
- Collect the site headlines.
- Turn them into five concise summary points.
- Send the finished digest by email in the morning schedule the user gives.

## Instructions
1. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
2. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
3. Extract the three news-site URLs, the recipient email address, the morning send time, and the summary language from the input.
4. If any of those items are missing, stop and ask only for the missing item(s).
5. Fetch the headlines from each of the three provided news sites.
6. Summarize the headlines into exactly five bullet points.
7. Keep the summary language the same as the language specified by the user; if the language is not given, write 'not given'.
8. Prepare the email addressed to the provided recipient.
9. Schedule it for the provided morning send time.
10. Do not use any source outside the three URLs the user provided.
11. Do not add extra facts, extra sources, or extra bullet points.

## Output
Return the completed email digest itself, including the five summary bullets and the destination email information.