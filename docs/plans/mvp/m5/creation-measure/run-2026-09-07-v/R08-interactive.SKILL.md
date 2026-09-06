---
name: return-request-7day-reply
description: When a customer submits a return request, check whether it is within 7 days and generate the corresponding standard Chinese reply letter. Use when you need a consistent reply based only on the dates and reason in the request text.
---

# Instructions

You handle a customer return request by checking whether it is within 7 days and then producing the corresponding standard letter in Chinese.

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Process

1. Read the return request text the user gives you.
2. Identify the request date and the purchase or receipt date if both are present.
3. Determine whether the request is within 7 days based only on the dates in the input.
4. Output the standard letter that matches the result.
5. If the input does not provide a required date, write 'not given' for that missing item and still produce the finished artifact only if the missing item does not make the letter impossible.

## Output requirements

- Write the letter in Chinese.
- Output only the letter itself.
- Do not include analysis, reasoning, bullet points about the rules, or commentary.
- Keep the content standard and fixed for the within-7-days case and the over-7-days case.
- Do not use outside information.
- Do not invent names, dates, policies, or compensation details.

## Decision rule

- If the request is within 7 days, write the within-7-days standard letter.
- If the request is more than 7 days after the purchase or receipt date, write the over-7-days standard letter.
- If the dates are insufficient to decide, write a brief Chinese letter stating that the needed date information is not given.

## Standard letter pattern

Use a concise customer-service tone. Include only the response needed for the result determined from the input. Make the message consistent across runs when the same case is given.

## Quality check

Before finalizing, verify that the output:
- is in Chinese,
- contains only the final letter,
- matches the within-7-days or over-7-days result from the input,
- does not introduce facts not present in the input.