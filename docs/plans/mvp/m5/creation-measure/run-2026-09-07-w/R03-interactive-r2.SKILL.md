---
name: daily-news-headline-summarizer-emailer
description: When you need a daily workflow that reads headlines from three news sites, compresses them into five Chinese summary bullets, and prepares or sends them by email.
---

# Purpose
Create a reusable Agent Skill that, when given three news-site URLs and a recipient email address, gathers the visible headlines from those three sites, compresses them into exactly five concise Chinese summary bullets, and returns the finished email content or sends it when email delivery is available.

## Task
Run every morning. Read headlines from exactly three news-site URLs, summarize them into exactly five Chinese bullets, and prepare the email for the recipient address provided in the input.

## Inputs
- Three news-site URLs.
- One recipient email address.
- The request must contain all three URLs and the recipient address; if any are missing, stop and report which item is missing.

## Tool requirements
- Use a page-fetching or web-search tool to read the three news-site pages.
- If email sending is available in the environment, send the email; otherwise only output the ready-to-send email content.

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
Return exactly one of the following as the finished artifact:
- Sent email result with To, Subject, and Body; or
- Ready-to-send email content with To, Subject, and Body.

Body must contain exactly five Chinese summary bullets.

## Constraints
- Use only the provided URLs and recipient address.
- If a page does not expose headlines clearly, note that the headline source is not given rather than guessing.
- Keep the summaries grounded in the fetched page text.
- Do not add extra sources beyond the three URLs supplied in the input.