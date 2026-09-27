---
name: translate-customer-feedback-zh-hant-summary
description: Use this skill when you need to translate English customer feedback into Traditional Chinese and then summarize it into exactly three points.
---

# Translate English Customer Feedback to Traditional Chinese and Summarize in Three Points

Use this skill when the input is English customer feedback and the output needs to be a Traditional Chinese translation followed by a three-point summary.

## Instructions

1. Read the input as customer feedback written in English.
2. First output a complete Traditional Chinese translation as its own section, and place it before the summary; do not rewrite, polish, or summarize the original text first.
3. After the translation, write a summary in exactly three bullet points.
4. Keep the summary in Traditional Chinese. Each point in the summary section must be fully written in Traditional Chinese and must not mix in English sentences as the main content.
5. Preserve the important information that is clearly present in the original feedback.
6. Do not add analysis, recommendations, or extra formatting unless the input clearly requires it.
7. If the input contains multiple sentences or multiple feedback points, still translate the entire passage first, then produce exactly three summary points based on that same passage; do not translate only part of the text or merely rephrase a few sentences.
8. If the input is missing, unclear, or not in English, handle it with the best common default available and state the assumption used in the output.

## Output format

First give the translation section, then give the summary section; the summary section must contain exactly three bullet points, and no extra advice, comments, or supplementary content may be added beyond those three points.

## Requirements to follow

- When the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Notes

- Use concise, faithful translation.
- Keep names, product terms, and numbers unchanged unless the input clearly asks for localization.
- If the input includes ambiguity, choose the most direct interpretation that fits the text.