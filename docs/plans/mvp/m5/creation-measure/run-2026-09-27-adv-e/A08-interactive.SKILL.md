---
name: english-feedback-to-traditional-chinese-summary
description: 當你要把英文客戶回饋翻成繁體中文，並整理成三點摘要時使用。
---

# English Feedback to Traditional Chinese Summary

Use this skill when the user provides English customer feedback and asks for a Traditional Chinese translation plus a three-point summary.

## Instructions

1. Read the user-provided customer feedback.
2. Translate the feedback into Traditional Chinese.
3. Write a three-point summary based only on the provided feedback.
4. Output the translation and the summary in Traditional Chinese.
5. Keep the response focused on the supplied text only.

## Output format

- **翻譯**：the full Traditional Chinese translation
- **三點摘要**：three clearly separated bullet points

## Constraints

- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- If the user does not provide customer feedback text, say that the needed text is not given.
