---
name: personal-loan-application-evaluator
description: Evaluate a personal loan application from credit score and debt ratio, and use it when you need a one-pass decision of approve, manual review, or reject. Use this Skill for single-application underwriting checks that must apply strictest-rule precedence when multiple conditions match.
---

# Personal Loan Application Evaluator

Evaluate one personal loan application at a time using the input you are handed.

## Inputs
Use the credit score and debt ratio provided in the request. If the request gives multiple applications, evaluate each one separately in the order given.

If a needed setting is missing, use the common default, name what you chose, and finish the work rather than stopping. Do not invent any fact the input does not give; if a fact is missing, mark it as not given in the language of the output.

## Decision rules
Apply the rules below in this order of strictness:

1. **Direct reject** if credit score is below 600, or debt ratio is over 60%.
2. **Manual review** if credit score is between 600 and 699 inclusive, or debt ratio is over 40% and at most 60%.
3. **Direct approve** only if credit score is 700 or higher and debt ratio is 40% or lower.

If one application matches more than one rule, choose the stricter result for the applicant:
**direct reject > manual review > direct approve**.

## Output
Return the decision for each application clearly and directly.

Use these exact labels:
- 直接核准
- 需要人工複審
- 直接拒絕

If amounts or quantities belong together, give their total.

You cannot send, post, schedule, monitor, or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.