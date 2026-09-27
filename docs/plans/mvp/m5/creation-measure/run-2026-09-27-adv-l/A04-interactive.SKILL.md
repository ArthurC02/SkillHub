---
name: weekly-performance-notice
description: Create ready-to-send weekly performance updates for email and Slack when the user provides this week’s business figures and wants a Friday-afternoon announcement.
---

# Weekly Performance Notice Skill

Use this skill when the user wants a weekly performance update turned into content that can be sent by email to the whole department and posted in Slack on Friday afternoon.

## What to produce

Given one week’s performance figures, produce two finished artifacts:

1. An email-ready message for the whole department.
2. A Slack-ready announcement.

If the input already includes a preferred tone or format, follow it. If a required setting is missing, use the common default, say which one you used, and finish the work rather than stopping.

If the input asks for a sent, posted, scheduled, monitored, or fetched action, do not claim to perform it. Deliver the content ready to use and state plainly that sending or scheduling is left to the person.

## Working method

1. Read the user’s input and identify the week’s performance figures.
2. Keep the content limited to the figures and claims the input provides.
3. Write one version for email and one version for Slack.
4. Make both versions clearly about the current week.
5. Return the finished text directly.

## Content rules

- Never invent a fact the input does not give — no name, date, figure or event — and write `not given` only for such a missing fact.
- If the input makes two requirements impossible to meet together (a length limit and “keep everything”), keep the hard limit and say in one line what you left out — never drop it silently.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output shape

Return the two artifacts in a clear, copy-pasteable format. If the user supplied a deadline or schedule wording, reflect it in the wording without adding new facts. If the input is incomplete, make the missing part explicit as `not given` instead of guessing.

## Quality check

Before finishing, verify that:

- the email version is ready to send,
- the Slack version is ready to post,
- both versions refer to the same week,
- no unsupported facts were added,
- any omitted detail is explicitly marked `not given`.

If the user asks for something outside content generation, keep the response to the ready-to-use text and note that execution is left to the person.