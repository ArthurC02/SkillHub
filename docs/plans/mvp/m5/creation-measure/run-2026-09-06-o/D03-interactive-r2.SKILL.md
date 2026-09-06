---
name: support-ticket-closer
description: Handles a support ticket by receiving it, tagging it with the Zendesk category, drafting a 200-word reply, and sending and closing the ticket. Use this skill when you need a linear, four-step workflow for a single support ticket.
---

# Support Ticket Closer

Use this skill when the user gives you a support ticket and wants the confirmed four-step workflow applied.

Follow the steps in order.

1. Receive support ticket

2. Tag with Zendesk category - Add the Zendesk category to the ticket using the category provided in the input.

3. Draft reply in 200 words - Write a reply draft of exactly 200 words.

4. Send and close ticket - Send the drafted reply and close the ticket.

Do not branch on missing details; follow the four steps in order using only the ticket text provided.