---
name: support-ticket-zendesk-reply
description: Use when you need to turn an incoming support ticket into a tagged Zendesk ticket, a 200-word reply, and a closed ticket in one pass.
---

# Support Ticket Zendesk Reply Workflow

Use this skill when you are given a support ticket and need to process it according to the fixed four-step workflow.

## Workflow

1. **Receive the support ticket**
   - Read the subject, body, requester, and priority.
   - Treat the ticket content as the only source material for the response.

2. **Tag the ticket with a Zendesk category**
   - Assign the most fitting Zendesk category based on the ticket content.
   - If the input does not explicitly name a category, choose the closest reasonable Zendesk category from the ticket context and state it in the output.

3. **Draft a reply in 200 words**
   - Write a concise, helpful reply of exactly 200 words.
   - Base the reply on the ticket details provided.
   - Do not add unrelated process steps, policy commentary, or branching logic.

4. **Send the reply and close the ticket**
   - Present the final response as the completed ticket action.
   - Include both the drafted reply and the close action in the output.

## Output requirements

- Keep the workflow in the same order as the four diagram nodes.
- Do not introduce any extra decision points, alternate branches, or exception paths.
- Use Zendesk explicitly in the tagging step.
- Ensure the reply is exactly 200 words.
- Produce the result as a reusable skill response, not as a one-off explanation.

## Style

- Be direct and operational.
- Prefer a support-agent tone: clear, polite, and action-oriented.
- If the ticket text is the only input, rely on it without asking follow-up questions unless the ticket itself is missing.