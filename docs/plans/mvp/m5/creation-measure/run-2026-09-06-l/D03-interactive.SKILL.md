---
name: zendesk-ticket-flow-skill
description: Use this skill when you need to turn a simple Zendesk support-ticket workflow into a portable Skill that follows a fixed linear process: receive the ticket, tag it, draft a roughly 200-word reply, then send and close it.
---

# Zendesk ticket flow

## Purpose
Follow the confirmed linear support workflow exactly as given by the diagram. Use this Skill when the task is to transform or execute a simple Zendesk support-ticket process in four ordered steps.

## Workflow
1. **Receive support ticket**
   - Treat the incoming item as the support ticket to work on.
   - If the input does not include a ticket or ticket content, say that the required ticket input is missing and stop.

2. **Tag with Zendesk category**
   - Apply the Zendesk category label that best fits the ticket.
   - If the ticket content does not support a clear category, choose the most reasonable Zendesk category from the available context and state that it was chosen as the best fit.

3. **Draft reply in 200 words**
   - Write a reply of about 200 words.
   - Keep the reply professional, direct, and aligned with the ticket content.
   - Do not add steps, branches, or extra roles beyond what the diagram shows.

4. **Send and close ticket**
   - Present the drafted reply as the response to send.
   - Mark the ticket as closed in the workflow output.

## Output requirements
- Preserve the four diagram steps in order.
- Include the Zendesk tagging action explicitly.
- Keep the reply length around 200 words.
- Return the result as plain instructional text suitable for a portable Skill.
- Do not invent extra workflow nodes, conditions, or branches.

## If input is incomplete
- If the ticket text is missing, stop and ask for the ticket content.
- If the ticket target or Zendesk context is missing, state that the workflow cannot continue without it.

## Style
- Be concise and operational.
- Use the exact workflow order from the diagram.
- Where the diagram is silent, do not add assumptions beyond the minimum needed to carry out the four steps.