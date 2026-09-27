---
name: competitor-price-tracker
description: Track three competitors' public pricing pages for price changes and produce a ready-to-use change summary when the user needs a manual monitor or notification draft.
---

# Competitor Price Tracker

Use this skill when you need to monitor three competitors' public, accessible pricing pages for changes and produce a ready-to-use summary of what changed.

Follow these instructions in one pass.

1. Read the user input and identify the three competitors.
   - If the user gives website URLs, use those URLs.
   - If the user gives company names only, treat the names as the monitoring list and use the public pricing page that the input explicitly provides, or say the needed URL is not given.
   - When the user gives company names only and no URLs, first output a monitoring list for the three names, then stop and do not report any price-check result for them in that run.
   - Never invent a company name, URL, price, or page title.
   - Write `not given` only for a missing fact from the input.

2. Check each competitor's public pricing page.
   - Use only publicly accessible pages.
   - If a page is blocked, behind login, behind a paywall, or otherwise inaccessible, state that you could not read it.
   - Do not claim you fetched or reviewed content you could not access.

3. Compare the current price information with the prior price information included in the user input or available in the same request.
   - If a price has changed, you must include all three items in the bullet: the old price, the new price, and the page URL.
   - When a price has changed, the bullet must explicitly show the old price, the new price, and the page URL in the same bullet.
   - If the current price matches the prior price, state clearly in the bullet that no change was detected.
   - Use an explicit status label such as `no change` or equivalent wording that leaves no ambiguity.
   - If there is no prior price in the input, say that the previous price is `not given`.

4. Produce the output as a concise change summary.
   - List each competitor separately.
   - Include the page URL when it is given.
   - If there is a change, begin with a notification-style summary of the change, and do not make raw page content the main result.
   - If there is no change, say so plainly.

5. Do not ask follow-up questions unless the input is missing the minimum facts needed to continue.
   - When a needed setting is missing, use the common default, say which one you used, and finish the work rather than stopping.
   - For this skill, the common default for a missing output format is a plain text summary.

6. Keep the work limited to what the input provides.
   - Do not infer other pages to track.
   - Do not analyze pricing strategy.
   - Do not create alerts, schedule messages, or send notifications.
   - Deliver the finished summary itself in the output; sending or scheduling is left to the person.

7. Output shape.
   - Start with one short heading.
   - Then give one bullet per competitor.
   - Each bullet should include: competitor name, page URL if given, status, and changed price details if any.
   - End with a short note only if some required fact was not given.