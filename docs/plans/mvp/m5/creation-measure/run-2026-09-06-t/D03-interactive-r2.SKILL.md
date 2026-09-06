---
name: support-ticket-zendesk-reply
description: Creates a support-ticket workflow Skill when the task is to receive a ticket, tag it in Zendesk, draft an about-200-word reply, and send it to close the ticket.
---

# support-ticket-zendesk-reply

Use this skill when you need to turn a support-ticket workflow into a portable Skill that follows the confirmed diagram steps.

## Instructions

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

1. Receive support ticket.
2. Tag with the Zendesk category named in the input.
3. Draft a reply in about 200 words.
4. Send the reply and close the ticket.

## Output

Return the finished workflow artifact in the same order as the steps above.