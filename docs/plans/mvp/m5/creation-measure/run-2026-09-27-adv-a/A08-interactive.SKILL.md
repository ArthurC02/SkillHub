---
name: english-customer-feedback-to-traditional-chinese-summary
description: 翻譯英文客戶回饋為繁體中文，並在需要整理客訴、意見回饋或使用者評論時，提供三點摘要。
---

# english-customer-feedback-to-traditional-chinese-summary

Use this skill when you are given English customer feedback, customer comments, or review text and need to produce a Traditional Chinese translation plus a three-point summary.

## Instructions

1. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
2. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
3. Read the English customer feedback exactly as provided.
4. Translate the feedback into Traditional Chinese.
5. Summarize the same feedback into exactly three points.
6. Keep the translation and the summary faithful to the original meaning.
7. Do not invent details, causes, actions, or outcomes that are not stated in the input.
8. If the input is silent about a detail that would otherwise be needed in the translation or summary, write 'not given'.
9. Output in Traditional Chinese.

## Output format

- **繁體中文翻譯**
  - Provide the full translation first.
- **三點摘要**
  - Provide exactly three bullet points.
  - Each bullet should capture one key idea from the feedback.

## Quality rules

- Preserve tone and intent as closely as possible.
- Do not add commentary about the translation process.
- Do not ask follow-up questions if the input already contains enough text to translate.
- If the input is empty or missing the feedback text, say so directly in Traditional Chinese and write 'not given'.