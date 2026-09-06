---
name: daily-news-headline-summarizer-emailer
description: When you need a daily workflow that reads headlines from three news sites, compresses them into five Chinese summary bullets, and prepares or sends them by email.
---

# Purpose
Create a reusable skill that, every morning, reads headlines from exactly three news websites, condenses them into exactly five Chinese summary bullets, and sends them to the specified email address.

# When to use
Use this skill when the user provides three news-site URLs and one recipient email address and wants a morning headline digest in Chinese.

# Inputs
- Three news-site URLs.
- One recipient email address.

# Tool requirements
- The runtime must be able to fetch page content from the provided URLs.
- If an email-sending tool is available, use it to send the message.
- If no email-sending tool is available, produce the finished email content instead of claiming that it was sent.

# Output
Return either:
- a sent-email result, or
- a ready-to-send email body.

The email body must contain exactly five Chinese summary bullets based only on the headlines visible in the fetched pages.

# Constraints
- Do not invent news sources, headlines, summary angles, delivery times, or email addresses.
- Use only the three provided URLs and the provided recipient email address.
- If a headline is not clearly visible in the fetched page text, do not guess it.
- Keep the summaries grounded in the fetched page text and limited to the requested five bullets.

# Procedure
1. Read the three URLs and the recipient email address from the user request.
2. Fetch each URL.
3. Extract the headlines that are visibly present in the fetched page text.
4. Combine the headline information from all three sites.
5. Write exactly five concise Chinese summary bullets.
6. Format the email with the recipient address and the five bullets.
7. Send the email if an email tool exists; otherwise output the finished email content.