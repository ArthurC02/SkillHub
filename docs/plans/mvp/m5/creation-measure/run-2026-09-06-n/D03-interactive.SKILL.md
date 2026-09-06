---
name: support-ticket-workflow
description: Use this skill when you need to process a single support ticket in a fixed four-step Zendesk-style workflow: receive the ticket, tag it, draft a 200-word reply, then send and close it.
---

# Support ticket workflow

Use this skill when the input is a single support ticket and you need to process it in the confirmed four-step order.

## Instructions

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

1. **Receive support ticket**
   - Read the ticket text exactly as given.
   - If the ticket text is missing, say **not given**.

2. **Tag with Zendesk category**
   - Assign the most appropriate Zendesk category based only on the ticket text.
   - If the category cannot be determined from the input, write **not given**.

3. **Draft reply in 200 words**
   - Write a reply draft of about 200 words.
   - Base the draft only on the ticket text.
   - If the needed details are missing, use **not given** rather than inventing them.

4. **Send and close ticket**
   - State that the reply should be sent and the ticket closed.
   - If the input does not support sending or closing, write **not given**.

## Output

Return the finished workflow result for the ticket in the same four-step order.
Do not add extra steps, branches, or unrelated tasks.