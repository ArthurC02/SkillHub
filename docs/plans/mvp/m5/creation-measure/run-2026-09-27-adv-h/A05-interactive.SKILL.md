---
name: competitor-price-tracker
description: Track price changes on three competitor websites’ public pricing pages and summarize any change when you need a reusable skill for periodic price comparison across competitor sites.
---

# Competitor Price Tracker

Use this skill when you need to compare the public prices on three competitor websites and report whether any price text has changed since the last run.

## What to do

1. Read the three website URLs and any target product, plan, or pricing-page details in the input.
2. For each site, inspect the most relevant public pricing page.
   - If the user names a specific product, plan, or page, use that target.
   - If the user does not specify one, use the main public pricing page or the most prominent public price on the page.
   - Say which target you used for each site.
3. Extract the visible price text or numeric price for each site.
   - Compare only visible text in the page content you can access.
   - Do not infer exchange rates, taxes, hidden fees, or discounted totals that are not shown.
4. Compare the current prices against the previous run’s stored result, if any.
   - If a site’s price text changed, mark it as changed.
   - If a site’s price text did not change, mark it as unchanged.
5. Report the results for all three sites.
   - If any site changed, summarize the change clearly.
   - If none changed, say there is no change.
6. If a page cannot be accessed or no recognizable price can be identified, report that the site could not be confirmed.

## Constraints

- Do not invent facts the input does not provide.
- If a required setting is missing, use the common default, state the default you used, and finish the work.
- If the request asks you to send, post, schedule, monitor, or fetch something you cannot do directly, prepare the content ready to use and state plainly that sending or scheduling is left to the person.
- If a length limit conflicts with "keep everything," keep the hard limit and say in one line what you left out.
- Deliver the finished artifact itself, not a plan, not a description of the rules, and not a request for access.

## Output format

Return a concise comparison for all three sites. Use one block per site with:
- site name or URL
- target page or target price item used
- extracted price text
- change status: changed, unchanged, or could not confirm
- short note if something changed or could not be confirmed

Then give one final summary line for the overall result.

## Notes

- This skill only compares and summarizes price changes.
- It does not send notifications itself; any notification or scheduling is left to the person running it or to external automation.