---
name: clinic-drug-expiry-inventory
description: Use this skill when you need to turn pharmacy-reported clinic drug records into a table and identify items expiring within 30 days from today, including a reminder to prioritize use or return.
---

# Clinic Drug Expiry Inventory

Use this Skill when you need to turn pharmacy-reported clinic drug records into a table and identify items expiring within 30 days from today, including a reminder to prioritize use or return.

## What this Skill does

1. Read each drug record the user provides.
2. Keep the records that expire on or before 30 days from today, counting today as day 0 and including day 30.
3. Return a table of the matching items.
4. Include a short reminder that the items should be used first or returned.

## How to run it in one pass

1. Identify the date of execution as **today**.
2. Read every provided record as-is.
3. Compare each expiry date against the range from today through the 30th day after today, inclusive.
4. Keep only records in that range.
5. Output the result as a table.

## Output requirements

Return a table with at least these columns:

- Drug name
- Batch number
- Expiry date
- Stock quantity
- Reminder

For every matching row, the reminder should say that the item should be used first or returned.

## Date rule

- Include items whose expiry date is today.
- Include items whose expiry date is 1 to 30 days after today.
- Exclude items whose expiry date is after the 30th day from today.

## Input handling

- Expect multiple records in one user message.
- Each record should provide, at minimum, name, batch number, expiry date, and stock quantity.
- If a required fact is missing from a record, mark only that fact as not given in the same language as the output.
- Do not invent any name, date, number, or event that the input does not provide.

## Defaults and limits

- If the user does not specify a format, use a simple Markdown table.
- If the user does not specify wording for the reminder, use: `優先使用或退回`.
- If the user does not give a working-day length or another setting needed for processing, use the common default and state what you chose in the output.
- If the request conflicts with a hard limit, keep the hard limit and say in one line what was left out.

## Output language

Write the output, including labels, in the language of the input.

## Important

- Work every figure out step by step and add the result up once more before giving it.
- When listing amounts or quantities that belong together, give their total.
- You cannot send, post, schedule, monitor, or fetch anything. If the request asks for that, prepare the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output; do not provide only a plan or explanation of the rules.