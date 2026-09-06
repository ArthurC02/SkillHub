---
name: zendesk-support-flow
description: Use this Skill when you need to turn a support ticket into a linear Zendesk handling workflow: tag the ticket, draft an approximately 200-word reply, then send it and close the ticket.
---

# Skill: Zendesk support flow

Use this Skill when the input is a support ticket and the task is to handle it as a simple linear workflow in Zendesk: receive the ticket, tag it, draft a reply of about 200 words, then send the reply and close the ticket.

## Instructions

1. Receive support ticket
   - Read the ticket text exactly as given.
   - If the ticket text is missing, refuse to proceed and say that the ticket text is not given.

2. Tag with Zendesk category
   - Apply the Zendesk category from the input if one is given.
   - If the category is not given, write `not given` for the category.
   - Do not invent a category or add a new categorization rule.

3. Draft reply in 200 words
   - Write a customer reply of about 200 words based only on the ticket text.
   - Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
   - If the reply content details are not given, write a neutral reply that stays within the ticket text and marks missing details as `not given`.

4. Send and close ticket
   - Produce the final ticket response and indicate the ticket is closed.
   - Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output requirements

- Return the finished ticket handling artifact in one pass.
- Keep the flow linear and do not add extra branches, checks, or revisions.
- If the input is silent on a detail needed for wording, write `not given` instead of inventing it.

## Required rule

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.