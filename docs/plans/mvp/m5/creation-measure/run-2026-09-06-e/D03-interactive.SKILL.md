---
name: zendesk-ticket-reply-flow
description: Transforms a support ticket into a Zendesk-tagged reply draft and a close-ticket action when you need a simple, linear support workflow. Use it for source material that shows receiving a ticket, categorizing it in Zendesk, drafting a short reply, and closing the ticket without alternate branches.
---

# Zendesk Ticket Reply Flow

## Purpose
Use this Skill when a support workflow is a simple linear sequence: receive a ticket, tag it in Zendesk, draft the reply, and send or close the ticket.

## Inputs
- A support ticket or customer message
- Any known Zendesk category or tag convention, if available
- Any required reply-length constraint, if available

## Process
1. **Receive the ticket**
   - Read the customer issue and identify the core request.
   - Extract any order numbers, dates, product names, or other identifiers.

2. **Tag in Zendesk**
   - Apply the best matching Zendesk category or tag based on the provided taxonomy.
   - If no taxonomy is provided, state that the taxonomy is unspecified instead of inventing one.

3. **Draft the reply**
   - Write a response that addresses the issue directly.
   - If a 200-word requirement is given, follow it exactly only when the user has made clear it is a hard limit.
   - If the constraint is unclear, present the reply length as unresolved and avoid pretending the limit is confirmed.

4. **Send and close**
   - End with the action to send the reply and close the ticket.
   - If the source material does not specify a decision rule, do not add one.

## Output format
Return the result in four parts:
- Ticket summary
- Zendesk tag or note that the taxonomy is unspecified
- Reply draft
- Send/close action

## Constraints
- Keep the workflow linear.
- Do not invent alternate branches, approvals, or escalation paths.
- Do not guess at Zendesk taxonomy values.
- Preserve uncertainty when the source does not resolve it.
- Stay concise enough to be reused as a portable support-workflow Skill.

## Example behavior
For a message like: “My order arrived damaged and I need a replacement immediately. Order number 48291. Please help.”

Produce:
- a short ticket summary,
- a note that the Zendesk category depends on the available taxonomy,
- a reply draft aligned to the issue and any confirmed length constraint,
- and a final send/close action.