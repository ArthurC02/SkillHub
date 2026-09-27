---
name: personal-credit-decisioning
description: Evaluate a personal loan application from credit score and debt ratio, and use it when a single application must be classified as approve, manual review, or reject.
---

# Personal Credit Application Decisioning

Use this Skill to classify a personal credit application from the applicant’s credit score and debt ratio.

## What this Skill does

Given one application at a time, determine the final decision:
- `直接核准` / approve
- `需要人工複審` / manual review
- `直接拒絕` / reject

If more than one rule matches, choose the stricter result for the applicant.

## Decision rules

Apply these rules exactly:

1. If credit score is 700 or higher, and debt ratio is 40% or lower, the result is `直接核准`.
2. If credit score is from 600 to 699 inclusive, or debt ratio is over 40% and up to 60% inclusive, the result is `需要人工複審`.
3. If credit score is below 600, or debt ratio is over 60%, the result is `直接拒絕`.
4. If more than one rule applies, choose the stricter outcome in this order:
   `直接拒絕` > `需要人工複審` > `直接核准`.

## How to work

1. Read the applicant’s credit score and debt ratio from the input.
2. Check all three rules.
3. If multiple rules apply, select the strictest matching result.
4. Output only the final decision unless the user explicitly asks for an explanation.

## Input expectations

The input should contain at least:
- one credit score
- one debt ratio

If either value is missing, do not invent it; mark it as not given and proceed only with the values present.

## Output format

Return the final decision in the same language as the user’s request when practical.
If the user asks for multiple applications, return one decision per application in the same order.

## Notes

- Treat `700` as included in the approve threshold.
- Treat `40%` as included in the approve debt-ratio threshold.
- Treat `600` and `699` as included in the manual-review score range.
- Treat `60%` as included in the manual-review debt-ratio range.
- Treat values above `60%` as rejection.
- When the request includes amounts or quantities that belong together, give their total.
- You cannot send, post, schedule, monitor, or fetch anything; prepare the content ready to use and leave sending or scheduling to the person.
- Deliver the finished artifact itself in the output, not a plan or request for access.