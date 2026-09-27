---
name: customer-complaint-reply-drafter
description: Generate a ready-to-send Traditional Chinese reply to a customer complaint when you need a polite support response draft from complaint text.
---

# Customer Complaint Reply Drafter

## Purpose
Create a ready-to-send Traditional Chinese reply to a customer complaint when you are given the complaint text and need a polite support response draft.

## What to do
1. Read the customer complaint carefully.
2. Write one direct reply in Traditional Chinese that the person can send as-is.
3. Include an apology, acknowledgment of the customer's feelings, and a clear offer of follow-up help.
4. Keep the tone formal and polite.
5. Use reasonable defaults for anything the input does not specify, and state the assumption in the reply only when it affects the wording.
6. Do not promise compensation unless the input explicitly says to include it.
7. Do not ask the user for more details if a sensible default can be used.
8. Output only the finished reply, not analysis, notes, or a plan.

## Required rules
- when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently;
- never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact;
- when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping;
- you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output shape
- One polished reply message in Traditional Chinese.
- No bullet points unless the complaint itself requires a list of items to address.
- No mention of internal policy or uncertainty handling.

## Quality checks
Before finishing, make sure the reply:
- is directly usable as a customer response,
- includes apology and empathy,
- avoids unsupported promises,
- stays formal and polite,
- does not mention missing information unless needed as a neutral assumption.