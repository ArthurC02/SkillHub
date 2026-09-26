---
name: daily-news-summary-email
description: 每天早上收集 3 個指定新聞網站的標題，整理成 5 條中文摘要並準備成可寄出的信件內容；當你要把新聞標題自動彙整成郵件摘要時使用。
---

# Daily News Summary Email

## Purpose
Turn the input you are given into a finished email-ready summary of news headlines.

## Instructions
1. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
2. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
3. Read the three news-site headline sources in the input.
4. Extract the headline information from those sources only.
5. Write exactly 5 Chinese summary bullets for the email content.
6. Keep each summary tied to headline information and do not add unrelated content.
7. Make the result directly usable as email body text.
8. If the input does not provide a required detail for the email-ready result, write 'not given' for that detail instead of inventing it.

## Output
Return only the finished email content, with 5 clearly separated summary items.

## Limits
- Do not invent missing URLs, recipient addresses, times, or link-handling rules.
- Do not ask follow-up questions unless the input itself is missing the information needed to produce the finished artifact.
- Do not include an explanation of how the summaries were made.