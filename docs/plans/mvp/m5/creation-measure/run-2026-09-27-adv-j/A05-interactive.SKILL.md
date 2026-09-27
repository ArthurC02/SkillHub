---
name: competitor-price-tracker
description: Track three competitor official pricing pages for price changes and generate a notification-ready summary when any price changes are detected. Use this when you need to monitor public competitor pricing pages and report changes or unreachable pages.
---

# Purpose
Track three competitor official pricing pages, compare the latest fetched price information with the previously recorded state, and produce a notification-ready summary when any price changes are detected.

## Rules to follow
- When the input makes two requirements impossible to meet together (a length limit and "keep everything"), keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give — no name, date, figure or event — and write "not given" only for such a missing fact.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## What to do
1. Read the user's message and identify the three competitor URLs or brand names.
2. If the user gives brand names instead of URLs, use the official public pricing or product page that the user provided or that is plainly named in the input. If no page is given, say the target page is not given.
3. For each source, fetch the public page with `fetch_url`.
4. Extract the pricing information that is visible on the fetched page.
5. Compare the extracted pricing information against the prior state available in the conversation or current run context. If there is no prior state, report that a baseline is not given and summarize the current prices only.
6. If a price changed, name the source, the price item, the previous value, and the new value.
7. If no source changed, say that no price changes were detected.
8. If a source cannot be fetched or the page content is unavailable, mark that source as unreachable or not retrievable and do not invent a price.
9. Return a concise summary that is ready to be sent to the user.

## Output shape
Produce a short notification-ready summary with these parts when applicable:
- tracked sources
- changed prices
- unchanged sources
- unreachable sources
- baseline status if no prior state is given

## Constraints
- Only use information visible in the input or returned from fetchable public pages.
- Do not guess prices, item names, or dates.
- If a page is blocked, inaccessible, or does not expose the pricing information, say so plainly.
- Do not ask the user to send or schedule the message; produce the message content only.
- If the input is missing a working format or scope detail, use a plain notification summary as the default and state that default in the output.