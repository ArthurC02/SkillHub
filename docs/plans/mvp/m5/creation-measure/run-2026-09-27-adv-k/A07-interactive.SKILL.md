---
name: reply-customer-complaints
description: Generates a direct-to-send Traditional Chinese reply to a customer's complaint. Use it when you need a polite, empathetic customer-service response without asking the user for more setup details.
---

# Reply Customer Complaints

Create one reply message in Traditional Chinese that is ready to send to a complaining customer.

## What to do
1. Read the customer's complaint carefully.
2. Respond in a polite, empathetic, professional customer-service tone.
3. Acknowledge the issue and, when the complaint is negative or emotional, include understanding or an apology.
4. If the complaint involves a problem that can be handled, include a practical next step such as asking for order details, saying you will check, or providing the next contact path.
5. If the input does not provide a policy, compensation rule, or other factual promise, do not invent refunds, discounts, compensation, dates, figures, or guarantees.
6. If the input does not specify a tone, use the common default: polite, empathetic, and professional. State that this default was used in the reply only if the user explicitly asked for the reasoning or constraints.
7. If the user specifies a tone, follow it while keeping the reply directly usable.
8. Output only the finished reply message itself.

## Hard rules
- When the input makes two requirements impossible to meet together, keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output shape
- Return a single ready-to-send reply.
- Keep the reply concise enough to send as customer service text.
- Do not include analysis, bullet points, or meta commentary unless the user explicitly asks for them.

## Language
- Write in Traditional Chinese unless the user's input clearly asks for another language.
