---
name: zendesk-ticket-workflow
description: Use this skill when you need to turn a single support ticket into a linear Zendesk handling sequence: tag it, draft a 200-word reply, send the reply, and close the ticket. It is meant for straightforward tickets that should follow one fixed workflow with no branching.
---

# Zendesk Ticket Workflow

## Purpose
Handle one support ticket through a fixed four-step workflow:
1. Receive the support ticket.
2. Tag it with the correct Zendesk category.
3. Draft a reply of about 200 words.
4. Send the reply and close the ticket.

Use this skill when the task is a simple, linear support workflow and the user provides a ticket plus the Zendesk category to apply.

## Instructions

1. **Read the ticket**
   - Identify the customer issue and any important context already present in the ticket.
   - Treat the ticket as the only case to process.

2. **Apply the Zendesk category**
   - Tag the ticket with the Zendesk category supplied by the user.
   - Do not add extra categories unless they are explicitly provided in the ticket material.

3. **Draft the reply**
   - Write a clear, helpful customer reply of about 200 words.
   - Keep the reply focused on the issue in the ticket.
   - Use a professional tone and make the response ready to send.

4. **Send and close**
   - Send the drafted reply.
   - Mark the ticket as closed after sending.

## Output expectations
When using this skill, produce the workflow outcome directly in the same order:
- the category tag applied,
- the 200-word reply draft,
- the send-and-close completion state.

## Constraints
- Keep the workflow linear.
- Do not introduce decision trees, exceptions, or extra steps.
- Do not ask for additional inputs if the ticket and Zendesk category are already provided.
- If the ticket or category is missing, state exactly what is missing and stop.