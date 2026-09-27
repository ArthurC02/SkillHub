---
name: weekly-sales-notice-composer
description: Create weekly Friday afternoon sales-notice content when you need a reusable skill that turns provided performance figures into ready-to-send email and Slack announcements. Use it when the request is to prepare, not actually send, recurring department-wide updates.
---

# Weekly Sales Notice Composer

Use this Skill when the user asks for a recurring Friday-afternoon notice based on provided weekly sales figures, especially when the output must be ready for both email and Slack.

## What this Skill does
- Turns the input into a finished notice package.
- Produces both an email version and a Slack announcement version.
- Keeps the user's figures intact and does not invent missing facts.
- Prepares content for sending, while leaving actual sending or scheduling to the person.

## How to work
1. Read the user's input and identify the weekly performance numbers, audience, timing, and delivery channels.
2. Use the provided figures exactly as given. If a fact is missing, write `not given` only for that missing fact.
3. Draft an email version and a Slack version from the same source data.
4. Include that the notice is for Friday afternoon, since that is the trigger in the request.
5. If recipient names, channel names, or the exact sending setup are missing, use the common default for the format and state the assumption in the output.
6. Deliver the finished content directly in the output.

## Required handling rules
- When the input makes two requirements impossible to meet together (a length limit and "keep everything"), keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output format
Return the finished notice package with these parts:
- `Email subject`
- `Email body`
- `Slack message`
- `Assumptions used`

## Content rules
- Keep the tone professional and concise unless the user asks otherwise.
- Keep the figures and labels aligned with the user's wording when possible.
- If the user gives multiple metrics, preserve them all.
- If the user asks for department-wide distribution, keep the audience broad unless the input names a narrower list.
- If the user asks for scheduling, note that the skill prepares the content only and that actual scheduling is left to the person.

## When the input is incomplete
If the input does not include the figures or the audience, produce a usable template with clearly marked blanks and say what is missing in the assumptions section. Do not stop with questions.

## Quality check
Before finishing, verify that:
- both email and Slack versions are present,
- the Friday-afternoon trigger is visible,
- no unsupported facts were added,
- the output is ready to use immediately.