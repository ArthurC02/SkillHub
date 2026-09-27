---
name: customer-call-overtime-fee-calculator
description: Calculate customer service call overtime fees from a single call duration in minutes. Use this skill when you need to turn a call length into a fee based on tiered minute rates.
---

# Customer Call Overtime Fee Calculator

Compute the overtime fee for a single customer service call duration in minutes.

## What to do

1. Read the call duration in minutes from the input.
2. Apply the fee rules exactly:
   - The first 10 minutes are free.
   - Minutes over 10 and up to 30 cost 5 yuan per minute.
   - Minutes over 30 cost 8 yuan per minute.
3. Return the fee in yuan and show the calculation clearly.

## Calculation rules

Use these tiers:

- If the duration is 10 minutes or less, the fee is 0.
- If the duration is greater than 10 and at most 30, fee = (duration - 10) × 5.
- If the duration is greater than 30, fee = (20 × 5) + (duration - 30) × 8. For 45 minutes, fee = (20 × 5) + (15 × 8) = 100 + 120 = 228.

## Output requirements

- State the final fee in yuan.
- Show the tier breakdown used to reach the result.
- For 45 minutes, the final fee must be 228 yuan, with both tiers shown in the breakdown.
- Keep the result based only on the provided duration and the rules above.

## Notes

- Treat the input as minutes.
- If the input format is unclear, use the common default of whole minutes and explain that assumption.
- Do not add any extra charges not stated in the rules.