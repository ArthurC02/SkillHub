---
name: zendesk-ticket-reply-workflow
description: Creates a portable Agent Skill for a linear support workflow: receive a support ticket, tag it with the Zendesk category, draft a 200-word reply, then send the reply and close the ticket. Use this when you need the workflow captured as an agent-ready skill.
---

# Purpose
Turn the confirmed linear support workflow into an agent-ready Skill.

## Instructions
Follow the confirmed nodes in order and do not add any extra steps, branches, or conditions.

1. Receive support ticket.
2. Tag with Zendesk category.
3. Draft reply in 200 words.
4. Send and close ticket.

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output requirements
- Produce the skill as a portable Agent Skill.
- Keep the workflow linear.
- Preserve the exact order of the confirmed nodes.
- State the 200-word requirement explicitly.
- Treat the Zendesk category tag as required.
- If the input omits any needed detail, write 'not given' rather than inventing it.