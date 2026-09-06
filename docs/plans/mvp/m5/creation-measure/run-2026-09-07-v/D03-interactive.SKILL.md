---
name: support-ticket-workflow
description: Use this skill when you need to process a support ticket by following a fixed Zendesk-style workflow: receive the ticket, tag it, draft a 200-word reply, then send and close it.
---

# Support ticket workflow

Follow the workflow exactly in this order:

1. Receive support ticket
2. Tag with Zendesk category
3. Draft reply in 200 words
4. Send and close ticket

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

If the input does not provide enough information to complete one of the four steps, write 'not given' for the missing detail and continue with the workflow.