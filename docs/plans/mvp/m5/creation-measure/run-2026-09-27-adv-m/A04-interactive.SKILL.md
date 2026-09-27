---
name: weekly-performance-notification-copy
description: Generate a ready-to-send email and Slack announcement for weekly performance numbers when a user asks for Friday-afternoon distribution to the whole department. Use this when the input provides the performance figures and you need copy that can be pasted into email and Slack.
---

# Weekly performance notification copy

Create two ready-to-use outputs from the input you are given:
1. an email message to the whole department, and
2. a Slack announcement.

Use the following rules:

- When the input gives the weekly performance numbers, turn them into both messages in Traditional Chinese.
- When the input mentions Friday afternoon, treat it as the intended trigger context, but do not create or describe any scheduling system.
- If the input does not give a required fact, write `not given` only for that missing fact.
- If a setting the work needs is missing, use the common default, say which one you used, and finish the work rather than stopping.
- Never invent a fact the input does not give.
- When the input makes two requirements impossible to meet together, keep the hard limit and say in one line what you left out.
- You cannot send, post, schedule, monitor or fetch anything, so if the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output.

## Output to produce

Return:
- a short email subject,
- the email body,
- a short Slack announcement.

## How to write it

1. Read the weekly performance figures from the input.
2. Keep the department audience explicit in the email.
3. Keep the Slack message concise and suitable for an announcement.
4. Preserve every figure that is provided.
5. If the input includes a Friday-afternoon trigger, mention that this is the intended timing only if it helps the wording; do not add process details.
6. If any figure, audience detail, or timing detail is missing, use `not given` for that missing fact and keep going.

## Required style

- Write in Traditional Chinese.
- Make the copy ready to paste into email and Slack.
- Do not ask follow-up questions.
- Do not describe policy or internal steps.

## Output format

Use this structure:

### Email subject
...

### Email body
...

### Slack announcement
...
