---
name: self-storage-intake-eligibility-review
description: Use this skill when you need to classify a self-storage applicant’s move-in eligibility and required deposit/guarantor based on credit score, late-payment history, and any prior facility damage record.
---

# Self-storage intake eligibility review

Use this skill to evaluate an applicant for self-storage move-in eligibility and assign the correct risk grade and financial requirements.

## Inputs to gather
Collect these facts before deciding:
- Applicant credit score
- Number of late-payment incidents in the past 12 months
- Whether the applicant has any prior record of damaging storage facilities

If any input is missing, stop and ask for it. Do not guess.

## Decision order
Apply the rules in this order:

1. **Damage record override**
   - If the applicant has **any** prior record of damaging storage facilities, classify as **D级**.
   - Outcome: **reject rental**.
   - This rule overrides all other conditions, including credit score and late-payment history.

2. **Credit score and late-payment rules**
   - If credit score is **700 or above** and late-payment incidents in the past 12 months are **0**, classify as **A级**.
     - Outcome: **no deposit required**.
   - If credit score is **700 or above** and late-payment incidents in the past 12 months are **1 or more**, classify as **B级**.
     - Outcome: **deposit equals 1 month’s rent**.
   - If credit score is **below 700**, classify as **C级**.
     - Outcome: **deposit equals 2 months’ rent** and **a guarantor is required**.

## Output format
Return the result clearly and concisely with these fields:
- Grade: A级 / B级 / C级 / D级
- Decision: approved / rejected
- Deposit requirement: none / 1 month’s rent / 2 months’ rent
- Guarantor requirement: yes / no
- Reason: brief explanation of which rule applied

## Examples
- Credit score 720, late payments 0, no damage record → A级, approved, no deposit, no guarantor
- Credit score 710, late payments 2, no damage record → B级, approved, 1 month’s rent deposit, no guarantor
- Credit score 680, late payments 0, no damage record → C级, approved, 2 months’ rent deposit, guarantor required
- Any credit score, any late payments, damage record present → D级, rejected

## Notes
- Treat 700 as included in the A/B threshold.
- The damage record rule always takes priority.
- Do not add extra criteria beyond those listed here.
