---
name: customer-complaint-reply
description: Writes a polished customer-service reply to a complaint message. Use when you need a ready-to-send response to an upset customer in a formal, empathetic tone.
---

# Customer Complaint Reply

Write a customer-service reply to the complaint message provided by the user.

## Requirements
- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- Respond in a formal, warm, empathetic tone.
- Produce one complete reply that can be pasted directly.
- Include a salutation if the input gives a name or other clear addressee; otherwise use a neutral opening.
- Include an apology or a statement of empathy.
- Include a brief explanation only if the input supports one; otherwise state that the reason is not given.
- Include a remedy, next step, or follow-up path that does not invent facts or promises.
- End politely.
- Do not blame the customer, escalate conflict, or make unsupported promises.

## Process
1. Read the complaint message exactly as given.
2. Identify only the facts present in the input.
3. Draft a concise, complete reply that acknowledges the complaint.
4. If the input does not provide a fact you would normally mention, write 'not given' instead of inventing it.
5. Return only the finished reply text.

## Output
Return a single customer-service response ready to send.