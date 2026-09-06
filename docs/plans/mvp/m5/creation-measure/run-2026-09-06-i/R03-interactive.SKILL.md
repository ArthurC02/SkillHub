---
name: news-title-digest-emailer
description: 每天早上從三個新聞網站整理標題成 5 條摘要並寄到指定信箱；當你要把固定新聞來源自動彙整成每日電子郵件時使用。
---

# News Title Digest Emailer

## Purpose
Use this skill when you need a daily morning email that collects headlines from exactly three news sites, condenses them into five short summaries, and sends the result to the specified inbox.

## What to do
1. Read the three news site names or URLs, the recipient email address, and the morning schedule from the user input.
2. If any of those required inputs are missing, ask only for the missing item(s) and stop.
3. Open the three provided news sources and collect current headline material from them.
4. Select the most relevant headline items across the three sources and write exactly five concise summaries.
5. Keep the summaries grounded in the provided sources; do not invent news or add facts that are not supported by the source material.
6. Draft a sendable email message that includes:
   - the recipient email address,
   - the five summaries,
   - a clear indication that this is a daily morning digest.
7. Return the email content in a clean, ready-to-send format.

## Constraints
- Use only the three sources supplied by the user.
- Do not substitute other outlets unless the user explicitly provides them.
- Do not change the requested output shape: it must be one email with exactly five summaries.
- If source content cannot be accessed, report that the digest cannot be completed from the provided sources.

## Output shape
Produce a single email draft with:
- To:
- Subject:
- Body containing exactly five news summaries
- A short note that the digest is for the morning schedule

## Notes
- If the user specifies a time zone or email format, follow that instruction.
- If the user does not specify a time zone, treat the schedule as the user's stated morning time and do not guess a time zone.