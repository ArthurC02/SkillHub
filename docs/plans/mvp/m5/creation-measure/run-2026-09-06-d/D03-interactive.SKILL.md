---
name: support-ticket-responder
description: Use this skill when you need to process a support ticket by categorizing it in Zendesk, drafting a concise 200-word reply, and completing the send-and-close workflow in order.
---

# Support Ticket Responder

## Purpose
Use this skill to handle a support ticket from intake through closure when the workflow is: receive the ticket, tag it with a Zendesk category, draft a 200-word reply, then send and close the ticket.

## Workflow
1. **Receive the support ticket**
   - Read the ticket content and any available context.
   - Identify the issue, urgency, and any information needed to respond accurately.

2. **Tag the ticket with a Zendesk category**
   - Apply the most appropriate Zendesk category based on the ticket content.
   - If the correct category is not explicit, choose the closest supported category from the available Zendesk taxonomy.
   - Do not invent a category name.

3. **Draft the reply**
   - Write a reply of about 200 words.
   - Keep the reply direct, helpful, and aligned with the ticket context.
   - Include only information supported by the ticket or provided context.
   - If important details are missing, ask for them in the draft rather than guessing.

4. **Send and close the ticket**
   - Send the drafted reply.
   - Close the ticket once the reply has been sent.
   - Treat sending and closing as the final sequential completion of the workflow.

## Output expectations
- Provide the selected Zendesk category.
- Provide the 200-word reply draft.
- Indicate that the ticket is ready to send and close, or that it has been sent and closed if the environment performs those actions.

## Guardrails
- Do not add approval steps unless the ticket context explicitly requires them.
- Do not add extra workflow branches.
- Do not invent a Zendesk category, template, or tone rule that was not supplied.
- Keep the workflow linear and in the confirmed order.