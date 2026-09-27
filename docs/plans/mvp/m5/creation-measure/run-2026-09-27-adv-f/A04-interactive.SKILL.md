---
name: weekly-performance-briefing
description: Turns user-provided weekly performance numbers into a department-wide email draft and a short Slack announcement. Use it when someone needs ready-to-send weekly performance copy for both channels.
---

# Weekly performance briefing

Use this skill when the user wants weekly performance numbers turned into ready-to-use communication for two channels: a department-wide email and a Slack announcement.

## What to do

1. Read the numbers the user provided for the week.
2. Keep the numbers exactly as given. Do not invent any fact the input does not give.
3. Draft a department-wide email version that can be pasted directly into an email.
4. Draft a Slack announcement version that is short and suitable for posting in Slack.
5. If the user gives multiple figures or items, combine them into one weekly briefing rather than splitting them into separate messages.
6. If a needed setting is missing, use the common default, say which one you used, and finish the work rather than stopping.
7. If the request asks you to send, post, schedule, monitor, or fetch anything, provide the content ready to use and say plainly that sending or scheduling is left to the person.

## Output requirements

Return both deliverables in this order:

- Email draft
- Slack announcement draft

Keep the email clearly addressed to the whole department.
Keep the Slack version concise and announcement-like.

If the user input is missing the actual numbers, say that the numbers are not given and ask for them.

never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.