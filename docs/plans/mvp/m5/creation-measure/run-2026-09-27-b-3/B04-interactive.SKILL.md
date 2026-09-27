---
name: electricity-bill-room-share
description: 將本期電費依每個人的房間坪數比例分攤，當你拿到電費總額與各房間坪數資料時使用。
---

# Electricity Bill Room Share

Use this skill when you need to split a current electricity bill across people by the size of each person's room.

## What to do

1. Read the total electricity bill amount and each person's room size from the user's input.
2. Compute the sum of all room sizes.
3. For each person, compute their share with this formula:
   `person share = total bill × (person room size ÷ total room sizes)`
4. Round each person's share to two decimal places using normal rounding.
5. If rounding causes the total of the displayed shares to differ from the original bill, adjust the final displayed amount by the smallest necessary amount so the displayed total matches the bill.
6. Present the result in a clear table with, for each person:
   - name
   - room size
   - ratio or percentage
   - share amount
7. Also show the total bill, the total room size, and the final sum of the allocated shares.

## Required instructions

- when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently; never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- If the user input is missing a total bill, one or more room sizes, or a person's name, use 'not given' only for that missing fact and finish the calculation only with the data actually present.
- If the user asks for another format, use that format; otherwise use a simple markdown table.
- Keep the work self-contained and ready to copy.

## Output format

Use this structure:

- A short title.
- A table with the calculation.
- A one-line note if any rounding adjustment was needed.
- A final total line.
