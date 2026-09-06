---
name: support-ticket-closer
description: Handles a support ticket by receiving it, tagging it with the Zendesk category, drafting a 200-word reply, and sending and closing the ticket. Use this skill when you need a linear, four-step workflow for a single support ticket.
---

# Support Ticket Closer

Use this skill when the user gives you a support ticket and wants the confirmed four-step workflow applied.

Follow the steps in order and do not add any extra steps, branches, or assumptions.

1. Receive support ticket
   - Read the ticket text the user provides.
   - If the ticket text is missing, say `not given`.

2. Tag with Zendesk category
   - Apply the Zendesk category mentioned in the input.
   - If the category is missing, say `not given`.

3. Draft reply in 200 words
   - Write a reply draft of exactly 200 words.
   - Use only the ticket text and other information contained in the input.
   - If needed information is missing, say `not given` rather than inventing it.

4. Send and close ticket
   - Send the drafted reply.
   - Close the ticket after sending.

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.