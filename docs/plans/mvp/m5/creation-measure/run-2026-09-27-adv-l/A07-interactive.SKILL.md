---
name: customer-complaint-reply
description: Turn a customer complaint into a polite, empathetic Chinese reply that can be sent directly. Use this when you need a ready-to-send customer-service response to a complaint or criticism.
---

# Customer Complaint Reply

## What this skill does
Turn the user's complaint text into a ready-to-send Chinese reply that sounds polite, empathetic, and professional.

Use this skill when the input is a customer complaint, criticism, dissatisfaction note, refund request, delivery issue, or product/service grievance and the goal is to produce the reply itself, not commentary.

## Operating rules
1. Read the complaint carefully and identify the concrete problem the customer states.
2. Write one direct reply in Chinese that the customer service team or business can send as-is.
3. Default to a formal, polite, empathetic tone unless the input clearly asks for a different tone.
4. Include an apology or acknowledgment of the customer's frustration when appropriate.
5. Address the specific issue mentioned in the complaint when the input provides one.
6. If the complaint lacks details, use safe, general customer-service wording and do not guess the missing facts.
7. Do not add unrelated content, policy explanations, or analysis.
8. Do not promise compensation, fault, timing, or outcomes that the input does not support.
9. Output only the finished reply text.

## Required behavior for missing or limited information
- If the complaint does not provide enough detail, stay general and professional.
- If a key fact is missing, write it as `not given` only for that missing fact.
- When a working-day length, tone, or format is missing, use the common default, state which one you used, and finish the work rather than stopping.

## Required behavior when constraints conflict
- If the input makes two requirements impossible to meet together, keep the hard limit and say in one line what you left out — never drop it silently.

## Required behavior about facts and delivery
- Never invent a fact the input does not give — no name, date, figure or event — and write `not given` only for such a missing fact.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output shape
Return a single Chinese reply paragraph or a short multi-sentence message that can be pasted directly into a customer-service channel.

## Suggested structure
- Acknowledge the issue.
- Apologize or express empathy.
- Respond to the stated problem.
- Offer a safe next step if the input supports one.

## Example behavior
If the customer says the product arrived scratched and different from the website photos, respond with a polite apology, acknowledge the mismatch, and offer a next step such as reviewing the issue or asking for order details, without inventing a compensation promise.