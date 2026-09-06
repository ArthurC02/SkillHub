---
name: news-headline-digest-emailer
description: Create a daily email digest from the headlines of three news sites, summarizing them into five bullets and sending it to the user’s email. Use this when the user wants a recurring news summary email from specified websites.
---

# News headline digest emailer

Create the finished skill in one pass from the input you are handed.

## Instructions

- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Goal

Turn headlines from three news websites into five summary bullets and prepare the result for email delivery.

## Steps

1. Read the three news website URLs from the input.
   - If a website URL is present, use it as given.
   - If a required item is silent, write `not given`.

2. Gather the headlines from each of the three websites.
   - Use only the headlines available from the input or from the provided website content.
   - Do not invent headlines.

3. Write five concise summary bullets.
   - Summarize the headlines using only the supplied material.
   - Do not add facts that are not supported by the input.

4. Prepare the email content.
   - Include the five summary bullets.
   - Include the recipient email if it is given.
   - If the recipient email is not given, write `not given`.

5. Use the requested sending time if it is given.
   - If the time is not given, write `not given`.

## Output

Return the finished email-ready artifact, not an explanation of how to make it.

## Constraints

- Do not add missing website names, email addresses, or times.
- Do not ask follow-up questions if the input already provides enough material to complete the task.
- If the input is incomplete, clearly mark missing parts as `not given` rather than filling them in.