---
name: daily-news-headline-summarizer-emailer
description: When you need a daily workflow that reads headlines from three news sites, compresses them into five Chinese summary bullets, and prepares or sends them by email.
---

# Purpose
Turn three provided news-site homepage or headline-page URLs into five Chinese summary bullets and deliver the finished email content, or send it if email-sending is available in the environment.

## Operating rules
- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- If the input does not provide the three news-site URLs or the recipient email address, stop and say what is missing.
- Do not invent a news source, a summary angle, a delivery time, or a mail provider.

## Steps
1. Read the user’s request and extract the three news-site URLs and the recipient email address.
2. If any of those inputs are missing, report the missing item as 'not given' and do not continue.
3. Fetch each provided URL.
4. From the fetched page text, identify the headlines that are visibly present on the page.
5. Combine the headlines from all three sites.
6. Write exactly five concise Chinese summary bullets based only on those headlines.
7. Prepare the email content with the recipient email address and the five bullets.
8. If an email-sending tool is available in the runtime, send the email; otherwise output the finished email content itself.

## Output format
Return the finished artifact itself, not a process explanation.

If sending is possible, provide:
- To: the recipient email address
- Subject: a brief subject line based on the request
- Body: exactly five Chinese summary bullets

If sending is not possible, provide the same email content ready to send.

## Constraints
- Use only the provided URLs and recipient address.
- If a page does not expose headlines clearly, note that the headline source is not given rather than guessing.
- Keep the summaries grounded in the fetched page text.
- Do not add extra sources beyond the three URLs supplied in the input.