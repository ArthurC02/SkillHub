---
name: news-headline-digest-emailer
description: Create a daily email digest from the headlines of three news sites, summarizing them into five bullets and sending it to the user’s email. Use this when the user wants a recurring news summary email from specified websites.
---

# News headline digest emailer

## Purpose
Create a daily workflow for turning headlines from exactly three news websites into five summary bullets and sending the result by email.

## When to use
Use this skill when the user provides three news-site URLs, a recipient email address, and a daily send time, and wants a recurring email digest.

## Required inputs
- Three news-site URLs
- Recipient email address
- Daily send time

If any of these are missing, say which ones are missing instead of filling them in.

## Tool requirements
- A web-reading tool to fetch the current headline pages from the three URLs
- An email-sending tool to deliver the digest to the recipient

## Output
When the user provides three news-site URLs, a recipient email address, and a send time, output a concrete daily workflow in Markdown with these steps:
1. Read the three provided news-site URLs and fetch the latest headlines.
2. Extract the headlines from all three sites.
3. Summarize the collected headlines into exactly five bullets.
4. Send the resulting email to the provided recipient address at the provided time.

Do not describe alternatives or limitations in the output when all required inputs are present.

## Constraints
- Do not add missing website names, email addresses, or times.
- Do not invent extra sources or extra recipients.
- If all three URLs, the recipient email, and the send time are present, output the workflow directly.
- If any of those inputs are missing, mark the missing parts as `not given` rather than filling them in.