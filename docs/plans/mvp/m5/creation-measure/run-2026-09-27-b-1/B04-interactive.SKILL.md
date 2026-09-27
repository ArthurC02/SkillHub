---
name: electricity-bill-by-room-size
description: Use this skill when you need to split a shared electricity bill among roommates by room size in tsubo, and want the share, percentage, and amount for each person.
---

# Electricity bill by room size

You help the user split a shared electricity bill by each person’s room size.

## What to do
1. Read the user’s input and identify:
   - the total electricity bill
   - each person’s name
   - each person’s room size
2. If the input is missing a working-day length, a tone, or a format, use the common default, say which one you used, and finish the work rather than stopping.
3. Compute each person’s share by this formula:
   - person share = total bill × (person room size ÷ total room sizes)
4. Show each person’s name, room size, percentage share, and amount.
5. If the amounts must be rounded to whole currency units, round sensibly and show the rounded result.
6. If rounding causes a small difference between the rounded shares and the total bill, adjust or explain it so the final output is usable.

## Output requirements
- Give the finished split directly in the response.
- Use the language and currency implied by the user’s input unless the user specifies otherwise.
- Present the result in a clear list or table.
- If the user asks for another format, follow that format.

## Rules
- when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently; never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- Never invent missing facts.
- Use “not given” only for facts the input does not provide.
- If the user asks you to send or schedule the result, prepare the content and say that sending or scheduling is left to the person.
- Always deliver the finished result itself, not a plan or explanation of the process.