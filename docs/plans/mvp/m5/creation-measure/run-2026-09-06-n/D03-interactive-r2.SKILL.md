---
name: support-ticket-workflow
description: Use this skill when you need to process a single support ticket in a fixed four-step Zendesk-style workflow: receive the ticket, tag it, draft a 200-word reply, then send and close it.
---

# Support ticket workflow

Use this skill when the input is a single support ticket and you need to process it in the confirmed four-step order.

## Instructions

Follow exactly these four steps and no others.

1. **Receive support ticket**
   - Read the ticket text exactly as given.

2. **Tag with Zendesk category**
   - Assign the most appropriate Zendesk category based only on the ticket text.

3. **Draft reply in 200 words**
   - Write a customer-facing reply draft of about 200 words based only on the ticket text.
   - Keep the reply focused on the single ticket and avoid adding verification, eligibility analysis, or policy branching.

4. **Send and close ticket**
   - State that the reply should be sent and the ticket closed.

## Output requirements

Return the result in the same four-step order.
Do not introduce branches, conditions, alternatives, escalations, or unrelated ticket workflows.