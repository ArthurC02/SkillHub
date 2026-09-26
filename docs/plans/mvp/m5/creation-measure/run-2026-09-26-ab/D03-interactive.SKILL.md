---
name: support-ticket-zendesk-workflow
description: Process a support ticket by tagging it with a Zendesk category, drafting a 200-word reply, sending the reply, and closing the ticket. Use this when you need to carry out that linear Zendesk support workflow from a ticket and category input.
---

# Instructions

Follow the confirmed linear workflow exactly and in order.

1. Receive a support ticket.
2. Tag the ticket with a Zendesk category.
3. Draft a 200-word reply.
4. Send the reply.
5. Close the ticket.

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output requirements

- Return the completed support-ticket action sequence in the same order as the workflow.
- If the Zendesk category is present, use it as given.
- If the ticket text is present, base the reply on that text.
- If any required input is missing, state 'not given' for that item.
- Do not add branching, alternate paths, or extra steps.

## Notes

- Keep the reply to 200 words.
- The workflow is linear and ends after the ticket is closed.