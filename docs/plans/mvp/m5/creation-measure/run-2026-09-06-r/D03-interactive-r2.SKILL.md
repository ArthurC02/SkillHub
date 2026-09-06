---
name: zendesk-ticket-flow
description: Processes a Zendesk support ticket through the confirmed linear workflow: receive the ticket, tag it with a Zendesk category, draft a 200-word reply, then send and close the ticket. Use this when the input is a support ticket and you need a deterministic four-step handling skill.
---

# Zendesk ticket handling workflow

Use this skill only for the confirmed four-step linear workflow.

## Rules
- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- Do not add any analysis, troubleshooting, recommendations, follow-up questions, or extra sections; output only the four confirmed steps.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Steps
1. **Receive support ticket**
   - Read the ticket text exactly as provided.
   - Identify the issue, the customer’s request, and any explicit constraints that appear in the ticket.
   - If a detail is silent, write **not given**.

2. **Tag with Zendesk category**
   - Assign the most fitting Zendesk category from the ticket content.
   - Always include the exact phrase "Zendesk category tag" in the output for this step.
   - If the input does not provide a category name, state **not given** instead of inventing one.

3. **Draft reply in 200 words**
   - Write a reply draft of about 200 words.
   - Base the reply only on the ticket content.
   - Do not introduce new facts, promises, dates, or steps.
   - If a necessary detail is missing, say **not given** in the draft where that detail would otherwise be required.
   - If the sample input does not provide a detail, do not ask for it; write the reply using only the ticket text and, where needed, the words "not given".

4. **Send and close ticket**
   - Output a final handling result that reflects the ticket has been sent and closed.
   - Keep this step to the confirmed linear workflow only.

## Output format
1. **Receive support ticket** — summarise the ticket in one sentence.
2. **Tag with Zendesk category** — state the Zendesk category tag, or `not given` only if the input does not provide enough information to choose one.
3. **Draft reply in 200 words** — provide the reply draft.
4. **Send and close ticket** — state the send-and-close result in one sentence.
Do not add any other sections before, between, or after these four lines.

Keep the four steps in order and do not add any extra branches or steps.