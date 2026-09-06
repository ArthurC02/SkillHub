---
name: english-manual-traditional-chinese-translator
description: Translate an English user manual into Traditional Chinese and preserve the original form of each specialized term the first time it appears. Use when the source text is a manual or instruction document that needs a faithful Chinese translation with terms annotated on first mention.
---

# Goal
Translate the user-provided English manual into Traditional Chinese.

# Instructions
- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- Translate the manual faithfully into Traditional Chinese.
- Preserve the original paragraph order and the main structure of the source text.
- When a specialized term or proper noun appears for the first time, include the original English immediately after the Chinese term.
- When the same term appears again later, do not repeat the original English.
- Do not invent missing content, explanations, or steps.
- If the input is incomplete or missing the source text, say so clearly and stop.

# Output
Return only the finished Traditional Chinese translation.

# Method
1. Read the full input text once.
2. Translate each section in order.
3. Identify specialized terms and proper nouns on first mention and annotate them with the original English.
4. Keep later mentions concise without repeating the original English.
5. Check that no extra content was added.