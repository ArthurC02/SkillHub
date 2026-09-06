---
name: daily-news-summary-email
description: Use when you need to collect headlines from three news websites each morning, condense them into five concise summaries, and send the result by email. This skill covers the workflow and checks needed, but it cannot itself browse the live web or send email without the appropriate tools and credentials.
---

# Daily news summary email

## What this skill does
Create a morning digest from three news websites by extracting their headlines, selecting the most important items, condensing them into five short summaries, and preparing an email-ready message.

## Important limitation
This task requires live website access and email delivery. If the agent does not have browsing and email-sending tools, or does not have the recipient address and sending permission, it cannot complete the task end-to-end. In that case, gather the needed information and prepare the digest text for manual sending.

## Workflow
1. **Confirm the source sites**
   - Identify the three news websites to use.
   - If they are not explicitly provided, ask for them before proceeding.
   - Use the same three sites consistently unless the user asks to change them.

2. **Collect the morning headlines**
   - Visit each site and extract the current top headlines.
   - Prefer the homepage, top stories section, or latest news section.
   - Record the headline text and source site for each item.
   - If a site is unavailable, note that clearly and continue with the others.

3. **Select the five most relevant items**
   - Combine all collected headlines.
   - Remove duplicates and near-duplicates.
   - Prefer the most important, timely, and broadly relevant stories.
   - If there are fewer than five distinct headlines across the three sites, use all available distinct items and state that fewer than five were available.

4. **Write five concise summaries**
   - Summarize each selected headline in one short sentence.
   - Keep the tone neutral and factual.
   - Do not add unsupported details beyond what the headline or linked article clearly states.
   - If the headline is ambiguous, phrase the summary conservatively.

5. **Prepare the email**
   - Use a clear subject such as: `Morning News Digest`.
   - Structure the body with:
     - a brief greeting,
     - the date,
     - the five summaries as a numbered list,
     - a short source note listing the three sites used.
   - Keep the email concise and easy to scan.

6. **Send or hand off**
   - If email-sending tools and credentials are available, send the message to the recipient mailbox.
   - If not, return the final email text so it can be sent manually.
   - Never claim the email was sent unless the sending action actually succeeded.

## Quality checks
- Verify that each summary corresponds to a real headline from one of the three sites.
- Ensure the final digest contains exactly five summaries when five distinct items are available.
- Avoid sensational wording and opinion.
- Keep source attribution accurate.
- If the task is scheduled for every morning, use the current date in the digest and repeat the same workflow each day.

## Output format when handoff is needed
Provide:
- Subject line
- Email body
- Source sites used
- Any missing information or access limitations
