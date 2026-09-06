---
name: support-ticket-zendesk-workflow
description: Turns a support ticket into a Zendesk-tagged, 200-word reply workflow and is used when you need a portable agent prompt for the exact linear support-ticket process shown in the diagram.
---

# Support Ticket Zendesk Workflow

## Purpose
Use this skill when you need a portable agent workflow that follows a four-step support-ticket process: receive the ticket, tag it with a Zendesk category, draft a 200-word reply, then send and close the ticket.

## Inputs
- A support ticket or support request text.
- A Zendesk category value, if one is available.
- Any source facts needed to write the reply.

## Output
- A completed ticket workflow in the same order as the diagram.
- A 200-word reply draft.
- A note showing the Zendesk category used.
- A send/close action described without assuming a specific channel.

## Instructions
1. **Receive support ticket**
   - Read the support ticket text provided by the user.
   - If the ticket text is missing, stop and ask for the ticket content.

2. **Tag with Zendesk category**
   - Apply the Zendesk category from the user or source material.
   - If no category is provided, state that the diagram does not specify the category and use a placeholder only if the surrounding task supplies one.
   - Do not invent a category.

3. **Draft reply in 200 words**
   - Write a reply that is exactly 200 words.
   - Base the reply on the ticket content and any facts supplied with it.
   - If the needed facts are missing, state that the reply cannot be completed from the provided material and ask for the missing information.

4. **Send and close ticket**
   - Present the final action as sending and closing the ticket.
   - Do not assume whether sending means email, a ticket reply, or another channel.
   - If the channel is not specified, keep the action channel-agnostic in the output.

## Constraints
- Follow the four steps in order and do not add extra branches.
- Do not substitute your own Zendesk category.
- Do not invent reply content beyond the supplied ticket facts.
- Keep the send step generic unless the user explicitly names a channel.
- If the user provides a complete ticket and category, complete the workflow in one pass.

## Format
Return the result as a concise workflow summary with these parts:
- Ticket received
- Zendesk category applied
- 200-word reply drafted
- Ticket sent and closed

## Notes
- The diagram does not specify the exact Zendesk category.
- The diagram does not specify the exact reply content beyond the 200-word requirement.
- The diagram does not specify the sending channel, so keep that step generic.