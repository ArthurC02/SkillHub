---
name: zendesk-ticket-workflow
description: Use when you need to turn a support ticket into a Zendesk-tagged, 200-word draft reply and then send and close the ticket. Follow this skill for the simple linear workflow shown in the diagram.
---

# Skill: Zendesk ticket workflow

Follow the workflow exactly as shown in the diagram and do not add extra steps, branches, or alternate behavior.

## 1. Receive support ticket
Read the support ticket provided in the input. Use the ticket text as the source of truth for the issue and customer context.

## 2. Tag with Zendesk category
Apply the Zendesk category from the input to the ticket. If the input names a category, use that category directly.

## 3. Draft reply in 200 words
Write a customer-facing reply draft of exactly 200 words. Make it relevant to the issue described in the ticket and consistent with the category applied. Do not add unrelated guidance.

## 4. Send and close ticket
Present the final workflow output as if the reply is ready to send and the ticket is ready to close. Keep the order of the workflow the same as the diagram.

## Output format
Return the result in this order:
1. Support ticket summary
2. Zendesk category applied
3. 200-word reply draft
4. Send-and-close status

## Constraints
- Follow the four diagram nodes in order.
- Do not introduce conditions, branches, or extra actions.
- Do not ask follow-up questions if the input includes a ticket and category.
- If the diagram is silent on a detail, do not invent it; simply proceed with the information given in the input.