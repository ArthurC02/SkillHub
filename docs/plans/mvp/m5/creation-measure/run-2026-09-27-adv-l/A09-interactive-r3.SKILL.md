---
name: monthly-overtime-marker
description: Calculate each person’s monthly overtime hours from attendance records and flag anyone above 46 hours. Use this skill when you need a one-pass result that lists overtime by person and highlights over-threshold cases.
---

# monthly-overtime-marker

Use this skill when the input contains monthly attendance records and you need each person’s overtime hours calculated, with anyone over 46 hours clearly flagged.

## What to do
1. Read the input as the source of truth.
2. Extract each person’s monthly overtime hours.
3. Compare each person’s overtime hours to 46.
4. Mark only people whose overtime is strictly greater than 46 hours.
5. Return the finished result in the format best suited to the input, keeping one entry per person.

## Output requirements
- Use a fixed three-column table with the headers Name, Overtime Hours, and Over 46 Hours. For anyone whose overtime is strictly greater than 46, the Over 46 Hours column must contain exactly the label Over; for everyone else it must contain exactly the label Not over.
- Show each person’s name and overtime hours.
- Do not flag people at exactly 46 hours.
- Do not flag people below 46 hours.
- If the input provides enough data to identify multiple people, include them all in the result.

## Operating rules
- when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently; never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## If the input is incomplete
- Use the missing information only if it is given in the input.
- If overtime hours are not directly provided, compute them only from data present in the input.
- If the necessary data still is not given, state what is not given and return the best possible partial result without asking a follow-up question.