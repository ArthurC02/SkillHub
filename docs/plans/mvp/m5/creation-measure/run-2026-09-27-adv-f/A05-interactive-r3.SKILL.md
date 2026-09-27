---
name: competitor-price-change-notifier
description: Track specified competitor pricing pages and produce a ready-to-send change notification when prices differ from the last recorded value. Use it when you need a reusable Skill to monitor competitor prices and report changes in one pass.
---

# Competitor Price Change Notifier

Use this Skill when you need a finished notification that can be sent as-is about competitor price changes.

## What to do

1. Read the input and identify the competitors, their pricing page URLs, and the current or last recorded prices.
2. For each competitor, compare the page price with the provided recorded price.
3. Report each competitor separately.
4. If a price changed, mark it as changed and show the old price and the new price.
When the current price differs from the recorded price, write the status as '有變動' and include both the old price and the new price in the same competitor entry.
When the current price differs from the recorded price, the competitor entry must use the exact status text '有變動' and show both prices side by side in that same entry.
5. If a price did not change, mark it as unchanged.
When the current price matches the recorded price, write the status as '無變動' in the competitor entry.
When the current price matches the recorded price, the competitor entry must use the exact status text '無變動'.
6. Include the page source or URL in every competitor entry, and if a competitor is marked '有變動', repeat the same page source or URL on that changed entry so the change can be traced back to its page.
7. Output a single notification message that can be sent as-is.

## If input is incomplete

If the input does not include a competitor name, a pricing page URL, or both an old price and a current price for comparison, stop and return a single finished notification that lists every competitor with the fields required in Output format, using 'not given' only for the missing field and keeping the other fields filled from the input.

## Important rules

- never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact;
- when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping;
- you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

Do not label the message as a draft; produce the final notification text only.

## Output format

Use this structure:

- Competitor name
- URL
- Old price
- New price
- Comparison result: changed / unchanged

For each competitor, always provide all four fields above so the comparison result is explicit.

Then add a short overall summary at the end.

## Notes

- If the user gives exactly three competitors, cover all three.
- If the user provides multiple prices, use the price that is explicitly labeled as the current or last recorded price for comparison.
- Keep the wording concise and ready to send.