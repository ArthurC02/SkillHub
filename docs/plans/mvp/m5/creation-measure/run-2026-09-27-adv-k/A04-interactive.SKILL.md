---
name: weekly-sales-summary-broadcaster
description: Create a weekly sales summary for Friday afternoons when given performance data, and use it when the goal is to prepare both an email update for the whole department and a Slack announcement. It formats the content only and does not send messages.
---

# Weekly Sales Summary Broadcaster

Use this skill when the user asks for a weekly Friday-afternoon sales or performance summary to be prepared for the whole department as both an email update and a Slack announcement.

## What this skill does

- Read the performance data the user provides.
- Summarize the current week.
- Produce two finished outputs:
  - a formal email version for the whole department
  - a concise Slack announcement version
- If the input includes numbers that can be totaled, compared, or turned into rates, calculate them directly.
- If required settings are missing, use the common default, say which default was used, and continue.

## Hard rules

- When the input makes two requirements impossible to meet together, keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Input you should expect

The user may paste:

- dated weekly sales or performance rows
- totals, targets, or prior-week figures
- a preferred audience name if they want one
- a preferred tone if they want one

If the input does not include an item needed for formatting, choose the common default and state it in the output.

## Procedure

1. Read the user's pasted data.
2. Identify the week covered by the input if it is given. If it is not given, use the week implied by the dates or labels in the input; if that is also missing, keep the summary generic and mark the week as not given.
3. Compute totals, subtotals, growth rates, and achievement rates only from the numbers present in the input.
4. If comparison data is present, compare it. If it is absent, do not invent a comparison; say it is not given.
5. Draft the email version in a formal tone, with a subject line suggestion and a clear body that can be sent as-is.
6. Draft the Slack version in a concise announcement style that can be pasted as-is.
7. If anything essential is missing for a calculation, state the missing field explicitly and do not guess.
8. Return the finished email and Slack copy directly.

## Output format

Return the content in this order:

1. `Email subject:`
2. `Email body:`
3. `Slack message:`

Keep the formatting clean and ready to use.

## Notes

- Do not claim that anything has been sent.
- Do not ask a follow-up question if you can complete the draft with defaults.
- Do not add facts that are not present in the user's input.