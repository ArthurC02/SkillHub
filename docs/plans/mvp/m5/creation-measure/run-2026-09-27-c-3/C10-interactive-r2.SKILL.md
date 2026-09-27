---
name: personal-loan-application-evaluator
description: Evaluate a personal loan application from credit score and debt ratio, and use it when you need a direct approve, manual review, or reject decision based on those thresholds.
---

# Personal Loan Application Evaluator

Use this Skill when you need to evaluate a single personal loan application from the applicant’s credit score and debt ratio, and return one of three decisions: `直接核准`, `需要人工複審`, or `直接拒絕`.

## Input

The user provides one application with:
- credit score
- debt ratio as a percentage

If the user gives multiple applications in one message, evaluate each one separately in the same order.

## Decision rules

Apply the rules below to each application:

1. If the credit score is 700 or higher and the debt ratio is 40% or lower, output `直接核准`.
2. If the credit score is between 600 and 699 inclusive, or the debt ratio is greater than 40% and no more than 60%, output `需要人工複審`.
3. If the credit score is below 600, or the debt ratio is greater than 60%, output `直接拒絕`.
4. If more than one rule applies to the same application, choose the strictest outcome for the applicant: `直接拒絕` takes precedence over `需要人工複審`, and `需要人工複審` takes precedence over `直接核准`.

## Output

Return only the decision for each application, using the exact terms above.

## Examples

- Credit score 720, debt ratio 35% → `直接核准`
- Credit score 700, debt ratio 40% → `直接核准`
- Credit score 650, debt ratio 30% → `需要人工複審`
- Credit score 720, debt ratio 50% → `需要人工複審`
- Credit score 590, debt ratio 30% → `直接拒絕`
- Credit score 720, debt ratio 65% → `直接拒絕`
- Credit score 580, debt ratio 50% → `直接拒絕`