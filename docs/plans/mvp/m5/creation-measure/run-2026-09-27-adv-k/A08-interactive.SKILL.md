---
name: english-customer-feedback-traditional-chinese-summary
description: Translate English customer feedback into Traditional Chinese and summarize it in exactly three points. Use this when you need a faithful Chinese translation plus a concise three-point summary of customer feedback.
---

# English Customer Feedback to Traditional Chinese Summary

Translate the input English customer feedback into Traditional Chinese, then provide a summary in exactly three points.

## Instructions

1. Read the full customer feedback provided in the input.
2. Translate the feedback into Traditional Chinese.
3. After the translation, write a summary with exactly three bullet points.
4. Keep the summary faithful to the original meaning; do not add new claims.
5. If the input contains multiple feedback items, translate them together and then summarize the combined meaning.
6. Preserve proper nouns, product names, and brand names as given when translation would make them unclear.
7. Output only the finished translation and the three-point summary.

## Output format

- **繁體中文翻譯：**
  [translated text]

- **三點摘要：**
  1. [point one]
  2. [point two]
  3. [point three]

## Required behavior

- If the input is entirely in English customer feedback, translate it into Traditional Chinese.
- If the input includes multiple paragraphs or multiple comments, handle all of them in one pass.
- If the input is missing, the skill should stop and ask for the customer feedback text.

## Quality bar

- The translation should be complete and natural in Traditional Chinese.
- The summary should reflect the main ideas from the source text.
- The three points should be concise and not overlap unnecessarily.

## Notes

- Use Traditional Chinese only, unless a proper noun should remain in English.
- Do not invent details that are not in the source text.