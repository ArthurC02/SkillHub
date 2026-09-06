---
name: zendesk-ticket-reply-workflow
description: Turns a support ticket into a Zendesk-ready response workflow. Use this when you need a ticket tagged, a ~200-word reply drafted, and the send/close step summarized without adding extra workflow steps.
---

# Zendesk Ticket Reply Workflow

Use this Skill when you are given a support ticket and need a Zendesk-ready response workflow.

Follow the confirmed diagram nodes in order:
Do not add internal notes, escalation guidance, approval options, status ladders, alternative resolutions, or any optional future outputs; output only the tag, one reply draft, and one send/close section.

1. **Receive support ticket**
   - Read the ticket text you were given.
   - Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
   - If the ticket text is missing, say so and stop.

2. **Tag with Zendesk category**
   - Assign the most fitting Zendesk category tag based only on the ticket text.
   - If the category cannot be determined from the input, write 'not given'.

3. **Draft reply in 200 words**
   - Write exactly one customer reply of about 180–220 words.
   - Make it the only reply draft in the output; do not provide alternatives, variants, or optional versions.
   - Keep the reply grounded in the ticket text only.

4. **Send and close ticket**
   - Create a separate section titled **Send/close**.
   - In that section, give one final action summary for sending the reply and closing the ticket, with no ticket-status ladder or macro list.

Output the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

Deliver the result in this structure:
- **Zendesk tag:** the selected tag or 'not given'
- **Reply draft:** the ~200-word customer reply
- **Send/close:** the final action summary

Keep the response concise and directly usable in a support workflow.