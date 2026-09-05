---
name: support-ticket-zendesk-workflow
description: Use when handling a support ticket that must be tagged in Zendesk, answered in about 200 words, and then closed. This Skill guides a simple linear workflow and is appropriate when there are no branching decisions shown.
---

# Support Ticket Zendesk Workflow

Use this Skill when you need to process a support ticket in a simple linear flow with no branching logic.

## Workflow

1. Receive the support ticket.
2. Tag the ticket with a Zendesk category.
3. Draft a reply in 200 words.
4. Send the reply and close the ticket.

## Guidance

- Keep the workflow linear. Do not add decision points, exception handling, or alternate branches unless the task text explicitly provides them.
- If the Zendesk category value is not specified, record that it is unspecified rather than inventing a category.
- If the desired reply tone, template, or style is not specified, note that it is unspecified and keep the reply focused on the ticket content.
- Preserve the 200-word requirement when drafting the reply.
- Use this Skill only for the narrow support-ticket flow described above.

## Output expectations

When using this Skill, the agent should:

- identify the ticket,
- apply a Zendesk category,
- produce a 200-word reply draft,
- and complete the close-out step.

Do not introduce extra steps that are not in the confirmed workflow.