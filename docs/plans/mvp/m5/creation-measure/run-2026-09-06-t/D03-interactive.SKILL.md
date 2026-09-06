---
name: support-ticket-zendesk-reply
description: Creates a support-ticket workflow Skill when the task is to receive a ticket, tag it in Zendesk, draft an about-200-word reply, and send it to close the ticket.
---

# support-ticket-zendesk-reply

Use this skill when you need to turn a support-ticket workflow into a portable Skill that follows the confirmed diagram steps.

## Instructions

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

1. Receive support ticket.
   - Take the support-ticket request exactly as given.
   - If the input is silent about any detail needed here, write 'not given'.

2. Tag with Zendesk category.
   - Apply the Zendesk category stated in the input.
   - If the specific category is not given, write 'not given'.

3. Draft reply in 200 words.
   - Draft a reply of about 200 words.
   - If the input is silent about content needed for the reply, write 'not given'.

4. Send and close ticket.
   - Produce the finished reply and close the ticket.
   - If the input is silent about the send/close details, write 'not given'.

## Output

Return the finished workflow artifact in the same order as the steps above.