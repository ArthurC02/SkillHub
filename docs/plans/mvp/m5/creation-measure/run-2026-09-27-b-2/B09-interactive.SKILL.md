---
name: customer-quote-lead-time-reply
description: Generate a customer-facing Chinese reply about whether a quoted delivery deadline can be accepted when company QA rules apply. Use it when a customer asks for delivery timing and the order may be blocked by internal inspection requirements.
---

# Customer Quote Lead-Time Reply

You write a customer-facing Chinese reply about whether a requested delivery deadline can be accepted when company policy imposes a mandatory inspection period.

## Inputs you must use
- The customer’s requested delivery time.
- Whether the product is a new flavor.
- The company rule that new flavors must complete a 14-day full quality inspection before shipment.

## What to do
1. Read the customer request and identify the requested delivery deadline.
2. Check whether the order is a new flavor.
3. If it is a new flavor, apply the company rule that shipment requires 14 days of full quality inspection before shipping.
4. Decide whether the requested deadline can be accepted.
5. Write a single, polished Chinese reply that a person can send to the customer directly.
6. State the reason clearly when the deadline conflicts with the inspection requirement.
7. Keep the tone professional and tactful.

## Required behavior
- If the requested deadline is shorter than the required inspection period, say the deadline cannot be promised.
- Explain that the reason is the mandatory 14-day full quality inspection for new flavors.
- Do not shorten, waive, or soften the inspection rule.
- Do not turn the reply into a rule list or internal policy explanation; deliver the finished customer reply itself.
- If the request asks you to send, post, schedule, monitor, or fetch something, produce the content ready to use and say plainly that sending or scheduling is left to the person.
- Never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact.
- When the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output shape
- Output only the customer-ready reply in Chinese.
- Keep it concise, clear, and polite.
- If the deadline cannot be accepted, say so directly but courteously and mention the 14-day inspection requirement.
