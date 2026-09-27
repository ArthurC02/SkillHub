---
name: weekly-sales-notification-composer
description: Create ready-to-use weekly sales report content for Friday-afternoon email and Slack announcements when a user provides this week’s performance numbers and delivery targets.
---

# Weekly Sales Notification Composer

Produce the finished weekly sales notification content in one pass from the input you are given.

## What this skill does

- Turn a user’s weekly sales or performance numbers into a clear weekly report.
- Prepare matching content for email and Slack when both destinations are requested.
- Preserve the user’s stated delivery timing, audience, and channel names.
- Output the finished artifact itself, ready for the person to send, post, or schedule manually.

## Use this skill when

- The user asks for a weekly sales update, performance summary, or revenue report.
- The user wants the same content prepared for email and Slack.
- The user asks for Friday-afternoon delivery wording or scheduling language.

## Operating rules

- when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently; never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Required behavior

1. Read the input and extract the weekly performance numbers, the intended audience, the delivery channel, and the timing.
2. If the input says “weekly Friday afternoon” but does not give an exact clock time, use the common default of Friday 3:00 PM and state that this default was used.
3. If the input names “all departments,” keep that audience as a whole department-wide audience in the output.
4. If both email and Slack are requested, produce both versions in a coordinated way so they clearly correspond to the same update.
5. If destination details such as an email address or Slack channel are missing, write them as not given rather than inventing them.
6. If the user asks you to send, post, schedule, monitor, or fetch, do not claim to do it; instead provide the content ready to use and say that sending or scheduling is left to the person.

## Output format

Return the finished content directly, using this structure:

- **Email subject**
- **Email body**
- **Slack message**
- **Timing note** if an exact time was not provided
- **Destination note** if any delivery target is not given

## Content rules

- Keep the email and Slack versions aligned in meaning.
- Use the exact figures provided by the user.
- Do not invent metrics, dates, recipients, or channel names.
- If the user supplies a recipient address or Slack channel, repeat it exactly.
- If the user does not supply one, mark it as not given.
- If the user’s input is too long and a length cap must be honored, keep the cap and state what was omitted in one line.

## Style

- Use a professional, concise tone by default.
- If the user does not specify tone, use a neutral workplace tone.
- Keep the output practical and ready to paste into an email client or Slack.

## Completion checklist

Before finishing, make sure you have:

- used only facts present in the input;
- preserved the weekly Friday-afternoon timing request;
- produced both email-ready and Slack-ready content when both are requested;
- avoided claiming to send or schedule anything;
- included not-given markers only for missing facts.
