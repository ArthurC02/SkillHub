---
name: competitor-price-tracker
description: Track three competitors’ official-site prices and emit a short notification when any price changes. Use this skill when you need a reusable prompt for comparing current public-page prices against a prior record and summarizing the changes.
---

# Competitor Price Tracker

Use this skill when the input provides three competitors, their official price-page URLs, and a prior price record, and the goal is to report whether any price changed.

## Instructions

1. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
2. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
3. Read the three competitor names, their price-page URLs, and the prior price record from the input.
4. Compare the current price information in the input with the prior record for each competitor one by one.
5. For each competitor, keep the name, current price, prior price, and source URL together.
6. If a price changed, state the competitor name, old price, new price, and URL in a short notification-style summary.
7. If no prices changed, still report each competitor separately, showing the name, current price, prior price, and a clear no-change status.
8. Do not ask for extra data if the input already includes the three competitors and their records.
9. Do not claim to support login-required pages, JavaScript-only pages, or any content that is not directly readable from the supplied input or public page text.
10. Keep the output concise and suitable for notification use.

## Output shape

- For every competitor, output one separate line or bullet that shows the competitor name, current price, prior price, and difference status.
- Do not collapse multiple competitors into one summary; always keep them separate.
- If a price changed, the competitor line must include the competitor name, old price, new price, and source URL.
- If nothing changed, still output one line or bullet per competitor; each line must clearly show the name, current price, prior price, and that there was no change.
- Never merge all competitors into one sentence or one combined summary; each competitor must stay separate with its own name and prices.
- Preserve the competitor names exactly as provided.
- Include the source URL for each reported competitor.

## Limits

Only use public pages whose content is directly readable without logging in. Do not claim support for login-required pages, JavaScript-only pages, or any content that requires interaction beyond the supplied text.

## Required behavior

When the input gives three competitor price entries and a previous record, compare them directly and produce the finished notification text in one pass.