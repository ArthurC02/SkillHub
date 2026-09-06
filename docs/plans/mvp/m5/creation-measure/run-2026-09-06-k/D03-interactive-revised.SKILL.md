---
name: support-ticket-workflow
description: Creates a portable Skill that follows a linear support-ticket workflow: receive the ticket, tag it with a Zendesk category, draft a roughly 200-word reply, and send and close the ticket. Use when you need a simple, branch-free ticket-handling procedure captured as an Agent Skill.
---

# Support Ticket Workflow

## Purpose
Follow this Skill whenever you need to process a support ticket in the confirmed linear sequence. Do not add branches, optional paths, or extra decision points.

## Steps
1. **Receive support ticket**
   - Start from the support ticket supplied by the user.
   - Treat the ticket as the only input to process.

2. **Tag with Zendesk category**
   - Apply an appropriate Zendesk category tag to the ticket.
   - Use the ticket content to choose the category if one is evident.

3. **Draft reply in 200 words**
   - Write a customer-facing reply of about 200 words.
   - Keep the reply concise, clear, and directly responsive to the ticket.
   - Preserve the request’s tone where appropriate.

4. **Send and close ticket**
   - Present the drafted reply as the final response.
   - Indicate that the ticket is sent and closed.

## Output format
Return the result as a simple four-step workflow in order:
- Receive support ticket
- Tag with Zendesk category
- Draft reply in 200 words
- Send and close ticket

## Constraints
- Do not introduce branching logic.
- Do not add alternate paths, escalation rules, or extra review steps.
- Keep the workflow linear and in the confirmed order.
