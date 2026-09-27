---
name: competitor-price-tracker
description: Track price changes on three competitor website product or pricing pages and report any differences from the previous check. Use this when you need a repeatable way to compare competitor prices and surface changes for a later notification workflow.
---

# Competitor Price Tracker

Use this Skill when you need to compare prices on three competitor website pages and report whether any price changed since the previous check.

## What to do

1. Take the user’s three competitor URLs or product/pricing page links as the source of truth.
2. Check each page and identify the main visible sale price on the page.
3. Compare each current price with the previous recorded price, if one exists.
4. Report the result for each URL in a clear per-site summary.
5. If any price changed, state the old price and the new price.
6. If no prices changed, say so explicitly and list all checked URLs.
7. If only some pages expose a readable price, separate the comparable URLs from the URLs that cannot be judged.
8. If the input does not provide exactly three URLs, continue with the URLs that were provided, state that only the provided URLs were compared, and state how many URLs are missing.

## Defaults and assumptions

- Use the main visible sale price on the page as the price to compare.
- If a page contains multiple prices, use the most prominent product or plan price unless the user has clearly identified a different target.
- Do not infer discounted, member-only, tax-exclusive, or region-specific prices unless the page labels them as the main price.
- If the working-day length or update cadence is missing, use the common default of a normal business-day workflow and finish the comparison instead of stopping.
- If a required setting is missing, state the default you used in the output.

## Output format

Return a concise change summary with these parts:

- URL
- current price
- previous price
- status: changed / unchanged / cannot determine
- short note if needed

If any price changed, include a one-line summary naming the changed URL(s) and the old/new prices.
If no price changed, include a one-line summary saying there was no change across the three checked URLs.

## Handling missing or blocked pages

- If a page is blocked, unavailable, or the price cannot be read, mark that URL as cannot determine.
- Do not guess the price.
- Do not invent page details.
- Keep the comparable URLs separate from the non-comparable ones.

## Limits

- You cannot send, post, schedule, monitor, or fetch anything outside the current run.
- When the user asks for notifications or ongoing tracking, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Do not claim that a notification was sent or scheduled.

## Writing rules

- When the input makes two requirements impossible to meet together, keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give; do not make up names, dates, figures, or events.
- Write "not given" only for a missing fact from the input.
- When a setting the work needs is missing, use the common default, say which one you used, and finish the work rather than stopping.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Required behavior

- Use the user’s provided URLs exactly as given.
- Compare the current page price against the previous recorded price when available.
- Report only what the page supports.
- If the user asks for notification delivery, provide the notification text or summary ready to send and state that sending is left to the person.
