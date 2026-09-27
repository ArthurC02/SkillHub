---
name: freight-shipping-fee-calculator
description: Calculate freight shipping fees from package weight using the stated rounding, tiered pricing, and cap rules. Use when the user asks for a shipping/freight cost based on weight.
---

# Freight shipping fee calculation

Follow these rules exactly.

## 1) Round weight up first
- Take the package weight in kilograms.
- Round it **up to the next whole kilogram** using ceiling behavior.
  - Example: 3.2 kg → 4 kg
  - Example: 5.0 kg → 5 kg

## 2) Apply the pricing tiers to the rounded weight
Use the rounded whole-kilogram weight:

- **5 kg or less**: charge **150**
- **More than 5 kg and up to 20 kg**: charge **250**
- **More than 20 kg**: charge **250 + 15 × (rounded weight − 20)**
  - Count every kilogram above 20 kg as a full kilogram.

## 3) Apply the maximum cap
- If the calculated fee is greater than **600**, charge **600** instead.

## 4) Output the result clearly
- State the rounded weight used for the calculation.
- State the final fee.
- If helpful, show the tier used and any cap applied.

## 5) If the weight is missing or unclear
- Ask for the package weight in kilograms before calculating.
- Do not guess.

## Examples
- 3.2 kg → rounds to 4 kg → **150**
- 5.0 kg → rounds to 5 kg → **150**
- 5.1 kg → rounds to 6 kg → **250**
- 20.0 kg → rounds to 20 kg → **250**
- 21.0 kg → rounds to 21 kg → **265**
- 50.0 kg → rounds to 50 kg → 250 + 15 × 30 = 700 → capped at **600**
