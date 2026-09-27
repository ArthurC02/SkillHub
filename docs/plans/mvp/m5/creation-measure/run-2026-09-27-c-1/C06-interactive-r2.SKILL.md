---
name: weekly-report-mailer
description: Generate a ready-to-send weekly report email for a manager and include weekly Friday scheduling guidance when the user wants recurring automation. Use this when you need a reusable skill that turns weekly status notes into a formal email draft and a repeatable weekly execution plan.
---

# Weekly Report Mailer

Create a ready-to-use weekly report email draft from the material the user provides, and include a weekly Friday scheduling plan when the user asks for recurring automation.

## What this skill does
- Turns weekly status notes into a formal email draft for a manager.
- Preserves the user’s facts and wording where useful, while making the message concise and professional.
- Adds a repeatable weekly Friday schedule when the user asks for automatic recurrence.
- Returns content ready to send or schedule, because this skill cannot send, post, schedule, monitor, or fetch anything itself.

## When to use it
Use this skill when the user wants:
- a weekly report rewritten as an email,
- a manager-facing status update formatted as a message,
- a Friday recurring workflow described clearly,
- or a single response that combines the email text and the schedule instructions.

## Instructions
1. Read the user’s input and identify the report content, recipient, subject line, and any recurrence request.
2. If a needed setting is missing, use the common default, name what you chose, and finish the work instead of stopping.
   - Default tone: professional and concise.
   - Default format: email draft with subject line, greeting, body, and closing.
   - Default schedule: every Friday.
   - Default output language: the same language as the user’s request.
3. Never invent a fact the input does not give. Do not make up names, dates, figures, or events.
   - If a fact is missing, mark it as not given in the output language.
4. If the input includes bullet points or raw notes, keep the facts and reorganize them into a polished report email.
5. If the input asks for recurring delivery, include a clear weekly-Friday schedule note in addition to the email draft.
6. If the request asks for sending or scheduling, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
7. If the user’s input combines amounts or quantities that belong together, give their total in the final output.
8. If the input asks for both the message and the schedule, include both in one response so the person can copy, send, or schedule it themselves.
9. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output shape
Return the result as:
- Subject:
- To:
- Body:
- Schedule note: only when recurring delivery was requested
- Missing information: only for facts not given

## Content rules
- Use the user’s provided report points as the source of truth.
- Keep the writing professional, clear, and brief unless the user asks otherwise.
- If the user asks to automate but no automation tool is available, provide the exact wording and schedule instructions that can be pasted into a mail or calendar tool.
- Do not claim that an email was sent or that a schedule was created.
- Do not ask follow-up questions once the user has asked to proceed or has already accepted the assumptions.

## Working example behavior
Input: a weekly report with completed work, next steps, recipient, and Friday automation request.
Output: a polished email draft plus a Friday recurrence note, both ready to paste into a mail client or scheduling tool.

## Reminder
When information is missing, use the common default, name the assumption, and continue. When the user wants automation, prepare the content and state clearly that actual sending or scheduling remains with the person.