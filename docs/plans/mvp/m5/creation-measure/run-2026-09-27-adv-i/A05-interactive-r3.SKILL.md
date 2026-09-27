---
name: price-change-monitor-notifications
description: Monitor three competitors’ public website prices and produce a ready-to-send change summary. Use this when you need to compare current prices against prior records and prepare notification text for any change.
---

# Purpose
Create a one-pass result for tracking price changes on three competitors’ official websites and preparing notification-ready text.

## Instructions
1. Read the user’s input and identify the three competitor website URLs, the target product or pricing pages, and any prior price records.
2. If a page path is missing, use the common default: the most obvious public pricing or product page reachable from the site’s homepage, and state that this default was used.
3. Fetch only public, openly readable page content.
4. Extract the current price from page text or parseable HTML.
5. If fetching or parsing a site fails, stop using any unverified price for that site and mark that site as unavailable with the specific reason for the failure.
6. When prior price data is present, compare it against the current price before choosing the status. If the current price differs from the prior price, mark the site as changed even when the fetch is partially limited, and do not replace that comparison result with unavailable unless the current price itself cannot be verified.
7. Compare the current price with the prior price record, if one is provided.
8. For each site, output:
   - the site or page name,
   - the current price,
   - the prior price when given,
   - whether the price changed,
   - a short notification-ready summary.
9. If a site cannot be read because of login, CAPTCHA, dynamic interaction, blocking, or missing parseable price text, report the limitation plainly and do not invent a price.
10. Finish the work in one pass. The output must be the finished monitoring summary itself, ready for the person to use.

## Required operating rules
- when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently;
- never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact;
- when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping;
- you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person;
- treat unavailable as a fetch-or-parse failure only; if the current price is visible and the prior price is given, prefer changed or unchanged and use unavailable only when no current price can be verified; never copy an unverified price into the current-price field;
- and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output format
Return a concise report with one section per website. Each section should include:
- URL or page name
- Current price
- Prior price, if supplied
- Status: changed / unchanged / unavailable, chosen in that order of preference based on the verifiable current price and prior price.
When a valid current price matches the prior price, the status must be unchanged; unavailable is only for cases where the current price cannot be verified at all.
- Notification text

If the user provided a requested notification channel, include a ready-to-send message for that channel. If no channel was provided, default to a plain text notification and say that it is ready to send manually. For any unavailable site, the notification text must clearly state the failure reason and that no price was provided.

## Limitations
- Only public website content is in scope.
- Do not claim a live alerting system exists.
- Do not fabricate prices or change history.
- If the site blocks access, explain that the result is unavailable.
- Sending or scheduling the notification is left to the person.