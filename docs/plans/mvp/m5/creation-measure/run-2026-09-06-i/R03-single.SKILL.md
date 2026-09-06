---
name: daily-news-summary-email
description: Use when you need a daily workflow that gathers headlines from three news sites, condenses them into five concise summaries, and sends them by email. This skill covers planning and drafting the automation, but it cannot itself browse the live web or send email without external tools and configured access.
---

# Daily news summary email

## What this skill does
Create a daily routine that collects headlines from three specified news websites, distills them into five short summary bullets, and emails the result to the recipient.

## Important limitation
This task requires live web access and email delivery. If those capabilities are not available in the current environment, do not pretend to complete the task. Instead, gather the missing details and prepare the exact workflow, prompts, or automation steps needed for a connected environment.

## Information to gather first
Before building or running the workflow, confirm:
1. The three news websites to monitor.
2. The recipient email address.
3. The sending method available in the environment:
   - SMTP credentials, or
   - an email API, or
   - a connected mail client/tool.
4. The delivery time and timezone for “every morning”.
5. Whether the summaries should be in Chinese, English, or both.
6. Whether the summaries should be neutral, business-focused, or tailored to a topic.

## Workflow
1. Visit each of the three news sites and collect the current top headlines.
2. Remove duplicates and near-duplicates across the sites.
3. Group related headlines into themes.
4. Write exactly five summary bullets that:
   - are concise,
   - reflect the most important items,
   - avoid speculation,
   - preserve factual meaning.
5. Draft an email with:
   - a clear subject line such as `Daily News Summary - YYYY-MM-DD`,
   - a short greeting,
   - the five summary bullets,
   - optional source links if available.
6. Send the email to the configured recipient.

## Summary rules
- Prefer the most important and widely covered stories.
- If fewer than five distinct themes exist, use fewer bullets rather than inventing topics.
- If more than five themes exist, choose the five with the highest relevance or impact.
- Keep each bullet to one or two sentences.
- Do not add opinions, predictions, or unsupported claims.

## Quality checks
Before sending, verify that:
- all three sites were checked,
- the five bullets are distinct,
- the email address is correct,
- the date and timezone are correct,
- the content is readable and free of duplicated wording.

## If automation is needed
If the user wants this to run automatically every morning, prepare a scheduled job in the available environment using the configured web and email tools. If scheduling is not available, explain that the agent can only prepare the workflow and cannot schedule or send it on its own.
