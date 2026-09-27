---
name: track-competitor-price-changes
description: Track three competitor websites’ prices and produce a change summary when compared against a previous baseline. Use this skill when you need a ready-to-use, notification-style report from provided price-page text or URLs, without inventing missing prices or fetching data that was not supplied.
---

# Track competitor price changes

Use this skill when you need to compare prices for three competitor websites and produce a concise change summary or notification-ready report.

## What you need to work with
- Three competitor sites, usually identified by their names and URLs.
- Price-page text or another user-provided source that contains the prices to compare.
- A previous baseline if one is available in the input. If no baseline is given, use the price values present in the input as the comparison basis and say that this is the basis used.

## Core job
1. Read the three competitors and the price information provided for each one.
2. Extract the main product price for each site.
3. Compare each site against the baseline available in the input.
4. Report any change clearly, including the site name, the field that changed, the old value, and the new value.
5. If nothing changed, say so plainly.
6. If a site cannot be compared because no usable price is given, mark it as not comparable instead of treating it as a change.

## Rules for handling the input
- When the input makes two requirements impossible to meet together, keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## How to compare prices
- Prefer the main product price shown on the page.
- Ignore shipping, taxes, and promotional copy that is not the actual sale price.
- If multiple prices appear, choose the main product price that best matches the offered product or plan.
- If the page does not make the main price clear, mark that site as not comparable.

## Output format
Return a clean, notification-ready summary in the same language as the input if possible.

Include:
- one line per competitor
- the current or observed price
- whether it changed
- if changed, the previous value and the new value
- if not comparable, the reason is 'not given' or a short explanation drawn from the input

A simple structure is acceptable:
- Competitor name: price
- Status: changed / unchanged / not comparable
- Details: old value -> new value, or 'not given'

## When the input includes multiple price items
- Compare only the item that represents the main price.
- Do not turn shipping, tax, or add-on prices into the main price.
- If the input contains several product prices on one site, use the clearest primary price tied to the main offer.

## If the user asks for a notification message
- Write the notification text directly.
- Keep it concise and ready to paste into chat, email, or a ticket.
- If a sending or scheduling action is implied, state that sending or scheduling is left to the person.

## If the input is missing required details
- Proceed with the common default available from the input.
- If a necessary value is not given, write 'not given' for that value.
- Do not ask follow-up questions inside the final answer.
- Finish the report with the information available.