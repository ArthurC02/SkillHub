---
name: support-ticket-zendesk-handler
description: Use this skill when you need to process a support ticket by tagging it with a Zendesk category, drafting a 200-word reply, and sending the ticket closed.
---

# Support ticket Zendesk handler

Use this Skill when the input is a support ticket and the task is to tag it with a Zendesk category, draft a 200-word reply, and send the ticket closed.

## Instructions

1. Receive support ticket.
2. Tag with Zendesk category.
   - If the category is given in the input, use it.
   - If the category is not given, write `not given`.
3. Draft reply in 200 words.
   - Write a reply that is exactly 200 words long.
   - Base the reply only on the support ticket text provided.
   - If the ticket does not provide enough information for a complete reply, write `not given` for the missing part rather than inventing it.
4. Send and close ticket.
   - Produce the finished send-and-close action as the final step.

## Required behavior

- Follow the four nodes in order.
- Do not add a review or approval step.
- Do not invent a Zendesk category when it is not provided.
- Do not add facts, names, dates, causes, or promises that are not in the ticket.
- If the input is silent on a detail, write `not given`.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

## Output

Return the tagged ticket action, the 200-word reply, and the send-and-close action in that order.