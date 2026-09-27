---
name: english-feedback-to-traditional-chinese-three-point-summary
description: Translate English customer feedback into Traditional Chinese and summarize it into exactly three points when you need a faithful bilingual response for review or reporting.
---

# English Customer Feedback to Traditional Chinese Three-Point Summary

## Purpose
Turn English customer feedback into a Traditional Chinese translation, then provide a concise three-point summary of the same feedback.

## When to use
Use this skill when the user provides English customer feedback and wants:
1. a Traditional Chinese translation, and
2. a summary of the main points in exactly three bullets.

## Instructions
1. Read the user’s English customer feedback carefully.
2. Translate the feedback into Traditional Chinese.
3. Summarize the feedback into exactly three points.
4. Keep the summary faithful to the source text. Do not add new facts, guesses, or outside context.
5. Present the translation first, then the three-point summary.
6. Use Traditional Chinese throughout the output.
7. If the input contains multiple sentences or multiple feedback points, include them all in the translation and compress them into three summary points.
8. If the input is missing or incomplete, still produce the best possible output from what is given and state that the missing fact is not given.
9. If the user asks for sending, posting, scheduling, monitoring, or fetching, provide the content ready to use and state plainly that sending or scheduling is left to the person.

## Output format
Use this structure:

### 翻譯
<Traditional Chinese translation>

### 三點摘要
1. <point 1>
2. <point 2>
3. <point 3>

## Constraints
- Keep the summary to exactly three points.
- Do not invent names, dates, figures, events, or causes that are not present in the source text.
- If a required setting is missing, use the common default, say which one you used, and finish the work.
- If two requirements conflict, follow the hard limit and say in one line what was left out.
- Deliver the finished artifact itself, not a plan or explanation of rules.