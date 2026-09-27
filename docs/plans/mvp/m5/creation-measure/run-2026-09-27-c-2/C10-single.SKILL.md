---
name: personal-loan-application-evaluator
description: Evaluate a personal loan application using credit score and debt ratio rules, and use it when you need a deterministic approve/review/deny decision from those two inputs. Apply the strictest matching rule when multiple conditions apply.
---

# Personal loan application evaluator

Use this skill to classify a single personal loan application into one of three outcomes:
- **Direct approve**
- **Manual review**
- **Direct deny**

## Inputs required
Collect these two values for the application:
- **Credit score**: a numeric score
- **Debt ratio**: a percentage value

If either value is missing, do not guess. Ask for the missing information before deciding.

## Decision rules
Evaluate the application against all applicable rules and choose the **most restrictive outcome** when more than one rule matches.

### 1) Direct deny rules
If **either** of the following is true, the application is **directly denied**:
- Credit score is **below 600**
- Debt ratio is **greater than 60%**

### 2) Manual review rules
If the application is not directly denied, and **either** of the following is true, the application requires **manual review**:
- Credit score is **between 600 and 699 inclusive**
- Debt ratio is **greater than 40% and up to 60% inclusive**

### 3) Direct approve rule
If the application is not denied or sent to manual review, it is **directly approved** when **both** of the following are true:
- Credit score is **700 or higher**
- Debt ratio is **40% or lower**

## Conflict handling
When multiple rules apply, use the stricter outcome in this order:
1. **Direct deny**
2. **Manual review**
3. **Direct approve**

Examples:
- Credit score 720, debt ratio 65% → **Direct deny** because debt ratio exceeds 60%
- Credit score 580, debt ratio 35% → **Direct deny** because credit score is below 600
- Credit score 680, debt ratio 30% → **Manual review** because score is between 600 and 699
- Credit score 710, debt ratio 45% → **Manual review** because debt ratio is above 40% and up to 60%
- Credit score 720, debt ratio 40% → **Direct approve**

## Output format
Return the decision clearly and briefly. Include the reason(s) that triggered the outcome.

Suggested format:
- Decision: Direct approve / Manual review / Direct deny
- Reason: short explanation of the matching rule(s)

## Important notes
- Treat the rules exactly as written.
- Do not average, score, or otherwise reinterpret the inputs.
- If the inputs are not numeric or the percentage is ambiguous, ask for clarification before evaluating.
