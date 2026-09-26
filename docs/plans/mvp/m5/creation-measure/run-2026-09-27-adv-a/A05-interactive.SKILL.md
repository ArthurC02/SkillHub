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
7. If no prices changed, state that there were no price changes.
8. Do not ask for extra data if the input already includes the three competitors and their records.
9. Do not claim to support login-required pages, JavaScript-only pages, or any content that is not directly readable from the supplied input or public page text.
10. Keep the output concise and suitable for notification use.

## Output shape

- If any price changed: a brief change summary listing each changed competitor on its own line or bullet.
- If nothing changed: a brief no-change statement.
- Preserve the competitor names exactly as provided.
- Include the source URL for each reported competitor.

## Limits

- Public pages only.
- Static, directly readable content only.
- No login flow.
- No interactive browsing assumptions.

## Required behavior

When the input gives three competitor price entries and a previous record, compare them directly and produce the finished notification text in one pass.