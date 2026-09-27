---
name: shipping-fee-reply-by-order-amount
description: Generate a one-sentence customer reply for shipping fees based on order amount. Use this when a prompt asks to explain shipping charges for specific order totals.
---

# Purpose
Create a one-sentence customer-facing reply that states the shipping fee based on the order amount.

# Inputs
- An order amount, given directly in the user's message.
- If the amount is not clearly stated, treat that fact as not given and do not invent it.

# Output
- Return exactly one sentence in Traditional Chinese.
- The sentence must state the shipping fee for the given amount.
- Do not add extra explanation, bullets, or headings in the final reply.

# Fee rules
- Less than 500: shipping fee is 80 yuan.
- 500 to 999 inclusive: shipping fee is 40 yuan.
- 1000 or more: free shipping.

# Procedure
1. Read the order amount from the user's message.
2. Decide which fee band applies.
3. Write one concise sentence that tells the customer the shipping fee.
4. If the amount is missing, use the common default of asking for the order amount is not allowed here; instead, state that the amount is not given and finish with a usable one-sentence template.

# Required response patterns
- For amounts below 500, say the shipping fee is 80 yuan.
- For amounts from 500 through 999, say the shipping fee is 40 yuan.
- For amounts of 1000 or more, say shipping is free.

# Constraints
- When the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently; never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# Notes
- Use the user's provided amount only.
- Do not infer discounts, taxes, or delivery regions.
- If the message contains multiple amounts, produce the one sentence that best answers the request for the set of amounts by reflecting the applicable fee rule for each amount only if the user explicitly asks for multiple replies; otherwise answer for the primary amount given first.