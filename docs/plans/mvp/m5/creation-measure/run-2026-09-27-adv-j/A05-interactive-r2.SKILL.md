---
name: competitor-price-tracker
description: Track three competitor official pricing pages for price changes and generate a notification-ready summary when any price changes are detected. Use this when you need to monitor public competitor pricing pages and report changes or unreachable pages.
---

# Competitor Price Tracker

Monitor three competitor pricing sources, compare the current price information with the latest stored baseline, and produce a concise notification-ready summary.

## When to use
Use this skill when you need to watch three competitor official pricing pages and report:
- price changes,
- no-change results,
- sources that cannot be fetched.

## Inputs
The user should provide three competitor URLs. If the user provides brand names instead of URLs, use the official public pricing or product page named by the user. If no page is named, treat the target page as not given.

## Required behavior
1. Treat all three supplied sources as tracked sources.
2. Fetch each public page with `fetch_url`.
3. Extract the visible pricing information from each page.
4. Compare the current price information against the most recent baseline available in the current task context.
5. If a price changed, report:
   - which competitor changed,
   - which price item changed,
   - the previous value,
   - the new value.
6. If none of the three sources changed, say clearly that no price changes were detected.
7. If a source cannot be fetched or does not expose pricing information, mark it as unreachable or not retrievable and do not invent a price.
8. Return a summary that is ready to send as a notification.

## Output format
Use short sections in this order when applicable:
- Tracked sources
- Changed prices
- Unchanged sources
- Unreachable sources
- Baseline status

## Constraints
- Only use information visible in the input or returned from fetchable public pages.
- Never guess prices, plan names, or dates.
- Never replace a failed fetch with a fabricated result.
- If the task has no prior baseline, say that the baseline is not given and summarize the current prices only.
- Keep the result concise and notification-ready.