---
name: weekly-sales-update-broadcast
description: 在每週五下午自動彙整本週業績數字，並同時寄送給全部門與在 Slack 公告；適合需要定期同步業績更新時使用。
---

# weekly-sales-update-broadcast

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Purpose

When the input asks to automatically share the current week’s sales numbers every Friday afternoon by email to the whole department and in Slack, produce the finished broadcast content or workflow text that preserves only those details.

## Instructions

1. Read the input exactly as given.
2. Keep the schedule as every Friday afternoon if that is stated.
3. Keep the data scope as this week’s sales numbers if that is stated.
4. Keep the audience as the whole department if that is stated.
5. Keep both delivery channels if they are stated:
   - email
   - Slack announcement
6. If any required detail is silent in the input, write `not given`.
7. Do not add any extra timing, numbers, recipients, metrics, or message content that the input does not provide.
8. Return the finished artifact itself, not an explanation of how to create it.

## Output shape

Produce a concise finished result that includes only the details present in the input:

- schedule
- data scope
- audience
- delivery channels

If the input does not provide a detail, write `not given` for that detail.

## Constraints

- Do not invent a subject line, message body, report format, or Slack wording unless the input gives it.
- Do not ask follow-up questions unless the input is missing the information needed to produce the finished artifact.
- Do not describe the rules or the process; output the artifact itself.

## When the input is incomplete

If the input is missing information needed to produce the artifact, state `not given` for the missing parts and still return the finished artifact shape.