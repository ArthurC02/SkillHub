---
name: parking-fee-calculator
description: Calculate parking fees from elapsed parking time. Use this when you need a quick rate-tiered parking charge result for a stay length in hours.
---

# Parking fee calculator

Compute the parking fee from the parking duration in hours.

## Rules
- Input is the elapsed parking time in hours.
- If the duration is less than or equal to 1 hour, charge 50.
- If the duration is greater than 1 hour and less than or equal to 3 hours, charge 120.
- If the duration is greater than 3 hours, charge 200.

## What to do
1. Read the input duration in hours.
2. Compare it against the three rate tiers.
3. Return only the fee amount for the matching tier.

## Output
- Return the calculated amount as the finished answer.
- Do not add extra rules, discounts, or time conversion steps.

## Acceptance behavior
- 0.5 hours -> 50
- 1 hour -> 50
- 2 hours -> 120
- 3 hours -> 120
- 4.75 hours -> 200

## Notes
- If the input uses decimal hours, treat them as valid.
- If the input is missing or unclear, use the common default of interpreting it as an elapsed duration in hours and finish the calculation with the information given.