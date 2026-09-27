---
name: freight-shipping-fee-calculator
description: Calculate freight shipping fees from package weight using the stated rounding, tiered pricing, and cap rules. Use when the user asks for a shipping/freight charge based on weight.
---

# Freight Shipping Fee Calculator

Use this skill to compute a single parcel’s freight fee from its weight.

## Rules to apply

1. **Round weight up first**
   - Always round the package weight **up to the next whole kilogram**.
   - Example: 3.2 kg → 4 kg.
   - Treat exact whole kilograms as unchanged.

2. **Apply the pricing tiers to the rounded weight**
   - **5 kg or less**: 150
   - **More than 5 kg and up to 20 kg**: 250
   - **More than 20 kg**: 250 plus **15 for each kilogram above 20 kg**
     - Use the rounded weight when counting the kilograms above 20.

3. **Apply the maximum cap**
   - If the calculated fee is greater than 600, charge **600**.

## Calculation procedure

1. Read the package weight.
2. Round it up to the next integer kilogram.
3. Determine the fee tier from the rounded weight.
4. If the weight is above 20 kg, add 15 per kilogram above 20.
5. Cap the final fee at 600.
6. Return the final amount in yuan.

## Examples

- 3.2 kg → rounded to 4 kg → 150
- 5.0 kg → rounded to 5 kg → 150
- 5.1 kg → rounded to 6 kg → 250
- 20.0 kg → rounded to 20 kg → 250
- 21.0 kg → rounded to 21 kg → 250 + 15 = 265
- 30.0 kg → rounded to 30 kg → 250 + 10×15 = 400
- 50.0 kg → rounded to 50 kg → 250 + 30×15 = 700 → capped at 600

## Response format

When asked to calculate, provide:
- the rounded weight
- the fee calculation
- the final fee after any cap

Keep the answer concise and numeric.
