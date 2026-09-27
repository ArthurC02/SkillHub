---
name: competitor-price-tracker
description: Track price changes across three competitor websites and produce a concise comparison summary when you need a one-pass skill for checking public competitor pricing. Use it when the user provides three competitor site URLs and wants changed, unchanged, or unavailable prices reported clearly.
---

# Competitor Price Tracker

Use this Skill when the user wants to compare prices on three competitor websites and get a concise summary of any changes. Work from the three URLs or site identifiers the user provides, and use public page content only.

## Inputs

- Three competitor website URLs.
- Optional page hints or product scope for each site.
- Any prior price values available in the input or accessible records.

If any of those are missing, use the common default: treat the provided URLs as the three competitors, compare only the public pages you can access, and summarize the prices visible in the current input or page content. Say which default you used in the output.

## Workflow

1. Read the three competitor URLs and any product or page hints.
2. Compare the public prices visible for each site.
3. Identify whether each site shows a price change, no change, or cannot be compared from the public page.
4. Produce a readable summary with one section per site.
5. If a price changed, include the site, the item or price field, the old price, the new price, and the amount or percentage of change when both values are available.
6. If no change is detected across all three sites, say that no price changes were detected.
7. If a site cannot be compared from the public page, say why in plain language.

## Output requirements

Return a finished comparison summary, not a plan and not raw page text. Keep the response directly usable by the person who asked for the tracking result.

## Required rules

when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently; never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Notes

- Use only what the input gives you or what is visible on the public pages.
- Do not invent competitor names, product names, prices, or dates.
- If the input includes only URLs and no prior prices, report what is visible now and state that a historical comparison is not given.
- Keep the summary concise and readable.