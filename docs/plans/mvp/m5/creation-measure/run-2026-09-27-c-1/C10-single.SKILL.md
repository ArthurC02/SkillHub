---
name: personal-loan-application-evaluator
description: Evaluate a personal loan application using credit score and debt ratio rules, and use it when you need a deterministic approve/review/deny decision from those two inputs.
---

# Personal Loan Application Evaluator

Use this skill to classify a single personal loan application into one of three outcomes:
- **直接核准**
- **需要人工複審**
- **直接拒絕**

## Inputs to collect
For each application, obtain:
- **信用分數**（credit score）
- **負債比**（debt ratio, %）

If either value is missing, do not guess. Ask for the missing value before deciding.

## Decision rules
Apply the rules below to the same application. If more than one rule matches, choose the **較嚴格、對申請人較不利**的結果.

### 1) 直接拒絕
Directly reject if **either** of the following is true:
- 信用分數 **低於 600**
- 負債比 **超過 60%**

### 2) 直接核准
Directly approve only if **both** of the following are true:
- 信用分數 **700 分以上**
- 負債比 **40% 以下（含 40%）**

### 3) 需要人工複審
Require manual review if the application is not directly rejected and **either** of the following is true:
- 信用分數 **600 到 699 分之間**
- 負債比 **超過 40% 但不超過 60%**

## Priority order
When evaluating, use this order:
1. Check **直接拒絕** conditions first.
2. If not rejected, check **直接核准** conditions.
3. Otherwise, classify as **需要人工複審**.

This priority ensures that when multiple rules apply, the stricter outcome wins.

## Step-by-step procedure
1. Read the applicant’s 信用分數 and 負債比.
2. If 信用分數 < 600, output **直接拒絕**.
3. Else if 負債比 > 60%, output **直接拒絕**.
4. Else if 信用分數 >= 700 and 負債比 <= 40%, output **直接核准**.
5. Else if 信用分數 is between 600 and 699 inclusive, or 負債比 is greater than 40% and at most 60%, output **需要人工複審**.
6. Otherwise, if none of the above conditions matched, default to **需要人工複審** because the application does not meet the direct-approval threshold.

## Output format
Return only the final decision label unless the caller asks for an explanation.

Valid labels:
- 直接核准
- 需要人工複審
- 直接拒絕

## Notes
- Treat boundary values exactly as written:
  - 600 is not below 600.
  - 700 counts as 700 分以上.
  - 40% counts as 40% 以下.
  - 60% does not count as 超過 60%.
- If the applicant meets both approval and review conditions, choose the stricter result only if the stricter rule is actually triggered; otherwise keep the direct-approval result.
- If the input is ambiguous or malformed, ask for clarification rather than inferring values.
