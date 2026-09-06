---
name: support-ticket-zendesk-handler
description: Use this skill when you need to process a support ticket by tagging it with a Zendesk category, drafting a 200-word reply, and sending the ticket closed.
---

# Support ticket Zendesk handler

Use this skill when you need to process a support ticket by producing a four-step workflow output that matches the source diagram: receive the ticket, tag it with a Zendesk category, draft a 200-word reply, and send/close the ticket.

## Instructions

1. Read the support ticket.
2. Output exactly four sections in this order: `Receive support ticket`, `Tag with Zendesk category`, `Draft reply in 200 words`, `Send and close ticket`.
3. In `Receive support ticket`, restate the ticket briefly so the workflow starts from the input itself.
4. In `Tag with Zendesk category`, state the Zendesk category being applied; if none is provided, write `not given`.
5. In `Draft reply in 200 words`, write one customer-facing reply of exactly 200 words.
6. Count only the reply text in the `Draft reply in 200 words` section; exclude headings, labels, and the other three sections from the word count.
7. In `Send and close ticket`, state that the ticket is sent and closed.

## Required behavior

- Follow the four sections in order.
- Do not add a review or approval step.
- Do not invent a Zendesk category when it is not provided.
- Do not add facts, names, dates, causes, or promises that are not in the ticket.
- If the input is silent on a detail, write `not given`.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.