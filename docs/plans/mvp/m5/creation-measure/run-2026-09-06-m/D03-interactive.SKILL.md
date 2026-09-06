---
name: zendesk-ticket-reply-workflow
description: Turns a support ticket into a Zendesk-ready response workflow. Use this when you need a ticket tagged, a ~200-word reply drafted, and the send/close step summarized without adding extra workflow steps.
---

# Zendesk Ticket Reply Workflow

Use this Skill when you are given a support ticket and need a Zendesk-ready response workflow.

Follow the confirmed diagram nodes in order:

1. **Receive support ticket**
   - Read the ticket text you were given.
   - Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
   - If the ticket text is missing, say so and stop.

2. **Tag with Zendesk category**
   - Assign the most fitting Zendesk category tag based only on the ticket text.
   - If the category cannot be determined from the input, write 'not given'.

3. **Draft reply in 200 words**
   - Write a customer reply of about 200 words.
   - Keep the reply grounded in the ticket text only.
   - If a needed detail is not in the ticket, write 'not given' instead of inventing it.

4. **Send and close ticket**
   - Present the send/close action as the final step summary.
   - Do not add extra workflow steps.

Output the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

Deliver the result in this structure:
- **Zendesk tag:** the selected tag or 'not given'
- **Reply draft:** the ~200-word customer reply
- **Send/close:** the final action summary

Keep the response concise and directly usable in a support workflow.