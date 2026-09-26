---
name: english-customer-feedback-to-traditional-chinese-summary
description: 翻譯英文客戶回饋為繁體中文，並在需要整理客訴、意見回饋或使用者評論時，提供三點摘要。
---

# english-customer-feedback-to-traditional-chinese-summary

Use this skill when you are given English customer feedback, customer comments, or review text and need to produce a Traditional Chinese translation plus a three-point summary.

## Instructions

1. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give.
2. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
3. Read the English customer feedback exactly as provided.
4. Translate the feedback into Traditional Chinese.
5. First output the complete Traditional Chinese translation, then output a three-point summary of the same feedback; do not begin with an English rewrite or any content other than the translation.
6. The summary must contain exactly three items and only those three items; do not add style options, rewrite suggestions, or other expanded content.
7. Keep the translation and the summary faithful to the original meaning.
8. Do not invent details, causes, actions, or outcomes that are not stated in the input.
9. The entire reply must be in Traditional Chinese; keep only the necessary English source text as material to translate, and do not include English introductions, explanations, or options.

## Output format

- **繁體中文翻譯**
  - Provide the full translation first.
- **三點摘要**
  - Provide exactly three bullet points.
  - Each bullet should capture one key idea from the feedback.
  - If there are fewer or more than three points, the output fails; each point must appear as its own bullet on a separate line.

## Quality rules

- Preserve tone and intent as closely as possible.
- Do not add commentary about the translation process.
- Do not ask follow-up questions if the input already contains enough text to translate.