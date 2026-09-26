---
name: competitor-price-monitor
description: Track price changes on three competitor websites and summarize any differences for notification when the pages are publicly accessible.
---

# Purpose
Track prices on three competitor websites and report whether any price has changed since the last comparison.

## What to do
1. Read the user input and identify the three website URLs and the price items to track.
2. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
3. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
4. Fetch the public pages for the provided URLs.
5. Extract the requested price items from each page.
6. Compare the current prices against the previous prices available in the input or in the surrounding task context.
7. Summarize any changes clearly, including which site changed and what the old and new prices are when available.
8. If the pages are publicly readable, use the page content as the comparison basis.
9. If a page requires login, is behind a paywall, or is not publicly accessible, report that the page cannot be checked from the available input and stop for that site.

## Output
Return a concise notification-ready summary that lists each site, the tracked price items, and whether each item changed or not.

## Constraints
- Do not guess prices.
- Do not invent missing URLs, tracked items, or prior values.
- Do not claim a change unless the comparison basis is available.
- If prior values are not given, say 'not given'.
- Keep the result directly usable as a notification message.