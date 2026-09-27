---
name: overtime-pay-calculator
description: Calculate monthly weekday overtime pay from an hourly wage and a list of daily overtime hours, especially when the input includes daily and monthly overtime caps.
---

# Overtime Pay Calculator

Calculate the monthly weekday overtime pay from an hourly wage and a list of daily overtime hours.

## Inputs
- Hourly wage.
- A list of daily overtime hours for one month.

## Calculation rules
1. For each day, the first 2 overtime hours are paid at 1.34 × the hourly wage.
2. For each day, overtime hours above 2 and up to 4 are paid at 1.67 × the hourly wage.
3. Any overtime beyond 4 hours in a single day is not paid.
4. Across the whole month, only the first 46 overtime hours are paid.
5. Any overtime hours beyond the monthly 46-hour limit are not paid.

## Procedure
1. Read the hourly wage and the list of daily overtime hours.
2. Process the days in the order given.
3. For each day, cap paid overtime at 4 hours.
4. Across the month, stop paying once the total paid overtime hours reach 46.
5. Apply the correct multiplier to each paid hour segment.
6. Sum all paid amounts and output the monthly overtime pay total.

## Output
- Return one total amount for the month.
- If the monthly limit removes any hours from payment, include only the paid total in the result.

## Notes
- Use the input as given; do not ask for extra information when the wage and daily hours are already provided.
- If the hourly wage or daily hours are missing, work from the material that is present and mark the missing fact as not given in the output.
- If the requested output must fit a hard length limit, keep the limit and say in one line what was left out.
- Do not invent names, dates, figures, or events that the input does not give.
- When a required setting is missing, use the common default, name what you chose, and finish the work.
- When listing amounts or quantities that belong together, give their total.
- You cannot send, post, schedule, monitor, or fetch anything; prepare the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output, not a plan or a request for access.