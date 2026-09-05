---
name: daily-news-title-summarizer-emailer
description: 每天早上從 3 個新聞網站擷取標題，整理成 5 條摘要並寄送到信箱；當你需要把多來源新聞標題定期彙整成郵件時使用。
---

# Daily News Title Summarizer Emailer

## Purpose
This skill turns the morning headlines from exactly three news websites into five concise summary bullets and sends the result by email. Use it when you need a repeatable workflow for collecting news titles, condensing them into a short briefing, and delivering that briefing to an inbox.

## Inputs
- Three news website URLs.
- The target email address.
- Any required email sending details available in the environment or connected mail tool.
- A morning trigger or schedule description.

## Core workflow
1. Collect the three source URLs.
2. Open each site and extract the current headline titles.
3. Combine the headline set and select the most relevant items.
4. Write exactly five summary bullets in Traditional Chinese.
5. Draft or send an email containing the five bullets and source attribution.
6. If a scheduling system is available, describe or configure the morning trigger separately from the news workflow.

## Operating rules
- Keep the output in Traditional Chinese.
- Preserve the fixed flow: collect headlines, summarize, then email.
- Do not assume any specific browser, scraper, email API, or scheduler.
- If fewer than three websites are provided, state that the intended source count is three and ask whether the workflow should be adjusted.
- If email configuration is missing, report the missing fields instead of pretending delivery is possible.
- If a morning schedule is requested, keep the schedule as a configurable trigger rather than hard-coding a particular platform.

## Output format
Produce a concise email-ready briefing with:
- A short subject line.
- Exactly five numbered summary items.
- A brief source list naming the three websites used.
- A short note when any required input is missing.

## Error handling
- If a source cannot be reached, report which site failed and continue with the remaining sources when possible.
- If the collected headlines are insufficient to make five distinct summaries, say so clearly and do not invent news.
- If the email target or sending capability is unavailable, stop before sending and list the missing information.

## What this skill does not do
- It does not choose or require a specific scheduler.
- It does not invent missing sources, email settings, or tool availability.
- It does not change the core task away from headline collection, summary writing, and email delivery.