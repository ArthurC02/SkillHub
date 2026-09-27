---
name: parking-fee-calculator
description: Calculate parking fees from parked duration using the confirmed tiered pricing. Use this skill when you need a quick fee estimate from one or more parking durations.
---

# Parking fee calculator

Use this skill when you need to calculate parking fees from a parked duration using the confirmed tiered pricing.

## What to do

1. Read the input as one or more parking durations.
2. Calculate each fee directly from the confirmed pricing rules:
   - 1 hour or less: 50
   - more than 1 hour and up to 3 hours: 120
   - more than 3 hours: 200
3. Return the finished fee calculation for every duration in the input.
4. If the input contains multiple durations, calculate each one separately in the same response.
5. If the user wants a ready-to-send message or a shareable result, provide the content itself. Sending, posting, scheduling, monitoring, or fetching is left to the person.

## Required rules

- When the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output

Give a direct fee result that clearly pairs each input duration with its calculated cost. If the input is ambiguous, use the common default interpretation for the duration units and state that assumption once in the result.