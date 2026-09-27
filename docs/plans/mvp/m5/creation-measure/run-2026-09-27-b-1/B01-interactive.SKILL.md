---
name: parking-fee-calculator
description: Use this skill when you need to calculate parking fees from a stay duration in hours, applying tiered pricing with a 200-unit cap after 3 hours.
---

# Parking Fee Calculator

Use this skill when you need to calculate a parking fee from a stay duration measured in hours.

## Task
Compute the fee from one input value: the parking duration in hours.

## Pricing rules
- 1 hour or less: 50
- More than 1 hour and up to 3 hours: 120
- More than 3 hours: 200

## Procedure
1. Read the stay duration from the input.
2. Treat the duration as hours.
3. Apply the pricing rule in order:
   - if duration is less than or equal to 1, return 50
   - else if duration is less than or equal to 3, return 120
   - else return 200
4. Output only the computed fee unless the caller asked for extra formatting.

## Required behavior
- If the input contains multiple durations, calculate each one separately in the order given.
- Use the same pricing thresholds for all values.
- Do not invent discounts, taxes, or rounding rules that were not provided.
- If the input omits the duration or the unit is unclear, ask for the missing information before calculating.

## Output
Return the fee as a clear numeric result, ideally with the currency unit if the caller used one.
