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

Do not introduce any branches, conditions, exceptions, follow-up paths, or “if/then” logic. Keep the workflow strictly linear.

## Output requirements
- Produce the skill as a portable Agent Skill.
- Keep the workflow linear.
- Preserve the exact order of the confirmed nodes.
- State the 200-word requirement explicitly.
- Treat the Zendesk category tag as required.