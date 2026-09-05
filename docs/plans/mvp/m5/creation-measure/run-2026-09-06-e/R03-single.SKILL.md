---
name: daily-news-summary-email
description: Use when you need a daily digest of headlines from three news sites summarized into five bullets and emailed to you. This skill covers gathering the headlines, condensing them into a short summary, and preparing the email content; it does not perform live browsing or send mail unless those tools are available.
---

# Daily news summary email

## What this skill does
Create a morning digest from three news websites by collecting their headlines, reducing them to five concise summary points, and preparing an email-ready message.

## Important limitation
If you do not have live web access, email-sending access, or the three target news sites are not specified, you cannot complete the full task autonomously. In that case, gather the missing details and prepare the digest template for the user to send or automate later.

## Procedure
1. **Confirm the three news sites**
   - If the user has not named them, ask for the exact site names or URLs.
   - If the sites are already known from context, proceed.

2. **Collect the latest headlines**
   - Visit each site’s homepage or news section.
   - Record the top headlines published for the morning digest.
   - Prefer the most recent, clearly labeled news items.
   - Avoid opinion pieces, ads, sponsored content, and duplicate stories across sites.

3. **Normalize and deduplicate**
   - Merge overlapping stories from different sites into one topic.
   - Keep the wording neutral and factual.
   - If a headline is ambiguous, use the article title and visible summary text to clarify the topic.

4. **Write exactly five summary bullets**
   - Combine the collected headlines into five concise points.
   - Each bullet should capture one major story or theme.
   - Keep each bullet short, readable, and suitable for email.
   - Do not invent facts beyond the headlines and visible article summaries.

5. **Prepare the email**
   - Use a clear subject such as: `Morning News Digest - YYYY-MM-DD`.
   - Include a brief greeting if appropriate.
   - Present the five bullets in a simple list.
   - Optionally add a short closing line.

6. **Send or hand off**
   - If email-sending tools are available and authorized, send the message to the user’s inbox address.
   - If not, return the finished email text so it can be copied into an email client or automation workflow.

## Quality checks
- The digest must contain exactly five summary bullets unless the user explicitly asks for a different number.
- The summary should reflect the morning’s headlines, not older or unrelated stories.
- Do not include unsupported claims, analysis, or personal opinions.
- If fewer than five distinct stories are available, combine related items into broader themes rather than padding with filler.

## If information is missing
Ask for:
- the three news websites,
- the recipient email address,
- the preferred send time and timezone,
- and whether the user wants only headlines or short summaries.

If live access or sending is unavailable, state that plainly and provide the best possible draft digest.
