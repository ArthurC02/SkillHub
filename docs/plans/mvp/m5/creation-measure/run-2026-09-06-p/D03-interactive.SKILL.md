---
name: zendesk-ticket-workflow
description: Turn a simple four-step support-ticket diagram into a direct workflow for handling intake, categorization, drafting, and closure. Use this skill when the user provides that workflow or a similar linear support process and wants it written as an actionable Skill.
---

# Zendesk ticket workflow

Use this skill when the input is a linear support-ticket workflow that matches the confirmed diagram.

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Procedure

1. **Receive support ticket**
   - Start with the ticket as given in the input.
   - If the ticket details are silent, write `not given`.

2. **Tag with Zendesk category**
   - Assign the Zendesk category stated in the input.
   - If the category is silent, write `not given`.

3. **Draft reply in 200 words**
   - Write the reply in exactly 200 words.
   - If the reply content is silent, write `not given`.

4. **Send and close ticket**
   - Send the drafted reply.
   - Close the ticket after sending.
   - If any send or close detail is silent, write `not given`.

## Output requirements

- Preserve the four steps in the same order as the input diagram.
- Do not add branches, conditions, checks, or extra steps.
- Do not invent missing facts; use `not given` when the input is silent.
- Return the finished workflow artifact directly.