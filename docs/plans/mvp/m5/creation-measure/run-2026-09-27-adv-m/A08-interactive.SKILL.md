---
name: customer-feedback-zh-tw-summary
description: Translate English customer feedback into Traditional Chinese and summarize it into three points when you need a concise bilingual customer-feedback digest.
---

# customer-feedback-zh-tw-summary

## Purpose
Translate English customer feedback into Traditional Chinese, then summarize the same content into exactly three points.

## When to use
Use this skill when the input is English customer feedback and the desired output is a Traditional Chinese translation followed by a three-point summary.

## Input
The user provides one or more sentences of English customer feedback.

## Output
Return:
1. A Traditional Chinese translation of the feedback.
2. A three-point summary in Traditional Chinese.

## Procedure
1. Read the feedback carefully and identify the original meaning.
2. Translate the feedback into Traditional Chinese.
3. Summarize the feedback into exactly three points.
4. Keep the summary faithful to the source and do not add facts, opinions, or details not present in the input.
5. Preserve the meaning of nuanced statements such as complaints, praise, mixed sentiment, and conclusions.
6. If the input contains multiple feedback statements, translate and summarize the full content, not just the first one.

## Output format
Use this structure:

**繁體中文翻譯**
[translated text]

**三點摘要**
1. [point one]
2. [point two]
3. [point three]

## Quality rules
- Use Traditional Chinese throughout the translated and summarized output.
- Do not use Simplified Chinese characters.
- Do not invent names, dates, figures, causes, or outcomes that are not given.
- If the source expresses uncertainty or mixed sentiment, reflect that uncertainty or mixed sentiment accurately.
- Keep the summary to exactly three bullet points or numbered points, no more and no fewer.

## Notes
- The finished artifact is the translation and summary itself.
- Do not ask follow-up questions unless the source text is missing or unreadable.