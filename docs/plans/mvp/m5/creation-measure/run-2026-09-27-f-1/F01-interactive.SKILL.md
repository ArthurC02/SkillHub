---
name: freight-fee-calculator
description: Calculate single-parcel freight fees from a weight input by rounding up to whole kilograms, applying tiered rates, and capping the result at 600; use it when you need a portable Skill to compute this shipping charge from a package weight.
---

# Freight fee calculator

Use this Skill when you need to calculate the freight fee for one parcel from its weight in kilograms, including decimal weights.

## What it does
- Rounds the input weight up to the next whole kilogram.
- Applies the confirmed rate rules.
- Caps the final fee at 600.
- Returns the fee as the final answer.

## Steps
1. Read one parcel weight in kilograms from the user.
2. Run `python scripts/calculate_freight_fee.py <weight>` from the directory that contains this `SKILL.md`.
3. Present exactly what the script prints.
4. If the user gave multiple weights in one message, run the script once per weight and present one fee per line in the same order.
5. If the input is missing, not a number, or otherwise unusable, the script prints a one-line error message and exits with code 2; present that message.
6. Do not change the fee, add extra charges, or infer missing business rules.

## Calculation rule used by the script
- Round the weight up to an integer kilogram.
- If the rounded weight is 5 kg or less, the fee is 150.
- If the rounded weight is more than 5 kg and 20 kg or less, the fee is 250.
- If the rounded weight is more than 20 kg, the fee is 250 plus 15 for every full kilogram above 20.
- If the computed fee is above 600, return 600.

## Output
- Output only the final fee value for each weight.
- For invalid input, output only the script’s one-line error message.
