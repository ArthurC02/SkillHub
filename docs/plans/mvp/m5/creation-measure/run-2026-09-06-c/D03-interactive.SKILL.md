---
name: support-ticket-workflow
description: Use when you need to process a support ticket by categorizing it in Zendesk, drafting a short reply, and then sending and closing the ticket. The skill is for straightforward ticket-handling flows where the workflow is fixed and unknown category or reply details must be left unspecified.
---

# Support Ticket Workflow

## Purpose
Use this skill when you need to process a support ticket through a fixed four-step workflow: receive the ticket, assign a Zendesk category, draft a 200-word reply, and send the reply before closing the ticket.

## Instructions
1. **Receive the support ticket.** Read the ticket content and identify the issue being reported.
2. **Assign a Zendesk category.** Choose the appropriate Zendesk category for the ticket.
   - Do not invent category names or assume a fixed category list.
   - If category values are not provided, leave the category decision as an explicit uncertainty.
3. **Draft the reply in about 200 words.** Write a concise customer-facing reply that fits the ticket.
   - Do not invent requirements for what the reply must include unless they are supplied in the ticket or by the user.
   - If reply-content rules are not provided, keep them as an explicit uncertainty.
4. **Send the reply and close the ticket.** After preparing the response, send it and close the ticket.
   - Do not add a human-review step unless one is explicitly provided.

## Uncertainties to preserve
Record these uncertainties whenever the source material does not resolve them:
- What the Zendesk category values are.
- What should be included in the 200-word reply.
- Whether any human review happens before sending and closing the ticket.

## Output expectations
When using this skill, produce a workflow result that follows the four confirmed steps in order and does not add extra steps or assumptions.

## Notes
- Keep the workflow aligned to the ticket only.
- Do not substitute invented policy, category labels, or approval rules for missing details.