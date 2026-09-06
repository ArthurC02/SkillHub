---
name: daily-news-headline-digest-emailer
description: Collects headlines from three news websites each morning, turns them into five concise summaries, and emails the digest to a specified inbox. Use this skill when you need a daily morning news email derived from website headlines.
---

# Daily News Headline Digest Emailer

## Purpose
Create a morning email digest from headlines collected from three news websites.

## Use when
Use this skill when you need an automated daily email summary based on headlines from three news sites and sent to a specified inbox each morning.

## Inputs
- Three news website URLs.
- One recipient email address.
- One morning send time.

## Output
Produce one email draft that is ready to send.

The email must include:
- `To:` with the provided recipient email.
- `Subject:` indicating a morning news digest.
- A body with exactly five numbered summary lines.

## Procedure
1. Read the three provided news website URLs.
2. Visit each site and collect available headline items.
3. Select five distinct headlines across the three sites.
4. Rewrite each selected headline into one short, neutral summary line.
5. Draft one email addressed to the provided recipient email.
6. Place exactly five numbered summary lines in the body.
7. Keep the content limited to headline-based summaries.
8. Do not add long article text or unsupported details.

## Quality rules
- Use only the supplied websites.
- Keep summaries concise and factual.
- Do not invent details not supported by the headlines.
- Preserve the recipient email and send time exactly as provided.
- Do not include full-article excerpts.
- Keep the output as a single email draft, not a report about the process.

## End state
A complete morning news email draft is produced for the given recipient and schedule.