---
name: customer-call-overtime-fee
description: 計算客服電話超時費；當你需要把通話分鐘數換算成費用時使用。
---

# Customer Call Overtime Fee

Use this skill when you need to calculate the fee for a customer service phone call from the call duration in minutes.

## Task

Compute the total fee in yuan from a single call duration given in whole minutes.

## Input

- One call duration in minutes.
- Treat the input as an integer number of minutes.
- If the input format is missing, use the common default of whole-minute input and finish the calculation.

## Pricing rules

- The first 10 minutes are free.
- Minutes over 10 and up to 30 cost 5 yuan per minute.
- Minutes over 30 cost 8 yuan per minute.

## Procedure

1. Read the call duration in minutes.
2. If the duration is 10 minutes or less, the fee is 0.
3. If the duration is greater than 10 and at most 30, charge 5 yuan for each minute above 10.
4. If the duration is greater than 30, charge 5 yuan for each minute from 11 through 30, plus 8 yuan for each minute above 30.
5. Output the result as minutes mapped to fee in yuan.

## Output

Return the fee in yuan for the given duration.

## Constraints and defaults

- Do not invent any missing fact such as a name, date, figure, or event; write `not given` only for a missing fact.
- When a setting the work needs is missing, use the common default, say which one you used, and finish the work rather than stopping.
- If the input makes two requirements impossible to satisfy together, keep the hard limit and state in one line what was left out.
- You cannot send, post, schedule, monitor, or fetch anything; if the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output, not a description of the rules, a plan, or a request for access.

## Examples

- 10 minutes → 0 yuan
- 11 minutes → 5 yuan
- 30 minutes → 100 yuan
- 31 minutes → 108 yuan