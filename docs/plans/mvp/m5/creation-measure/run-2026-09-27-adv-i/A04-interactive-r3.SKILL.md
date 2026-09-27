---
name: weekly-sales-announcement
description: Use this skill when you need a weekly Friday-afternoon sales summary prepared as an email and Slack announcement for the whole department. It turns provided weekly sales figures into ready-to-send copy and defaults missing audience details sensibly without claiming any sending occurred.
---

# Weekly sales announcement

Create the weekly Friday-afternoon sales email and Slack announcement from the input you are handed in one pass.

## Instructions

1. Read the input and identify the weekly sales figures, the intended email recipients, and the Slack destination.
2. If the input gives recipient or channel details, use them. If a setting needed for the work is missing, use the common default, say which one you used, and finish the work rather than stopping.
   - For audience defaults, use the whole department for email and the main Slack channel for the announcement when the input omits them, and say once that defaults were used.
   - If the input does not give a timezone or exact afternoon cutoff, use the system default workday afternoon interpretation.
3. Produce two finished artifacts:
   - an email body ready to send
   - a Slack post ready to paste
4. Include the weekly sales numbers clearly in both artifacts.
   - When sales figures are missing, the email and Slack post must both say that the figures are not given rather than implying the week was already summarized.
5. If weekly sales figures are missing, do not summarize performance as if numbers exist; instead add a short note that the sales figures are not given and, if the input names a source you can use, point to that source.
6. If the input asks for a send, post, schedule, monitor, or fetch action, do not claim to do it. Deliver the content ready to use and state plainly that sending or scheduling is left to the person.
7. If the input makes two requirements impossible to meet together, keep the hard limit and say in one line what you left out — never drop it silently.
8. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
9. Do not state that the email or Slack post has already been sent or published unless the input explicitly provides that result.

## Output format

Return the two artifacts with clear labels:

- `Email`
- `Slack post`

If you used a default for audience, timezone, or format, mention that once in a short note before the artifacts.

## Hard rules

- never invent a fact the input does not give — no name, date, figure or event — and write `not given` only for such a missing fact;
- when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping;
- you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Minimal content to include

- A concise subject line for the email.
- A short body that summarizes the week’s results.
- A Slack message that can be pasted as-is.
- The sales figures from the input, preserved accurately.
- A brief note only if a default was used or a required fact was missing.