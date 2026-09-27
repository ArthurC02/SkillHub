---
name: translation-job-classifier
description: Classify translation agency jobs by language pair, word count, and deadline; use it when you need to label requests as 退件, 加急, or 普通 from those three inputs.
---

# translation-job-classifier

## What this Skill does
Classify each translation agency job as `退件`, `加急`, or `普通`.
Use it when a request provides a language pair, a word count, and a deadline, and you need a single category for each job.

## Inputs
Each job must provide:
- language pair
- word count
- deadline in hours

If a required value is missing, keep that value as `未提供` in the output language.

## Decision order
1. Check the language pair first.
2. If the language pair is not `中文-英文` or `中文-日文`, classify the job as `退件`.
3. If the language pair is supported, then check the word count and deadline.
4. If word count is greater than 5000 and deadline is 48 hours or less, classify the job as `加急`.
5. Otherwise, classify the job as `普通`.

## Steps
1. Read each job item.
2. Compare its language pair against the supported pairs: `中文-英文`, `中文-日文`.
3. If unsupported, output `退件` for that item and stop checking the rest of its fields.
4. If supported, compare word count to 5000.
5. Compare deadline to 48 hours.
6. Apply the rule in the decision order above.
7. Output one classification per job item, in the same order as the input.

## Output
Return only the classification labels for the jobs, one per item, in order: `退件`, `加急`, or `普通`.