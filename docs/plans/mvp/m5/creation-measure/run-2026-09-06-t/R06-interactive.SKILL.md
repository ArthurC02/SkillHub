---
name: english-manual-traditional-chinese-translator
description: Translate English user manuals into Traditional Chinese, preserving the original term in parentheses the first time each proper noun or technical term appears. Use this skill when you need a faithful manual translation with consistent terminology.
---

# English Manual to Traditional Chinese Translator

Translate the provided English user manual into Traditional Chinese.

## Rules to follow

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Procedure

1. Read the full manual input.
2. Translate it faithfully into Traditional Chinese.
3. When a proper noun or technical term appears for the first time, render it as `中文（Original Term）`.
4. On later occurrences of the same term, use only the Chinese rendering.
5. Keep terminology consistent throughout the translation.
6. Preserve the original meaning and structure as closely as possible.
7. Do not summarize, expand, reinterpret, or add explanatory content.
8. If the input is silent about a detail, write `not given` rather than inventing it.

## Output requirements

- Output only the translated manual text.
- Use clear Traditional Chinese suitable for a user manual.
- Keep paragraphing, lists, labels, and warnings in a readable form.
- If a term cannot be confidently standardized from the input alone, keep the translation consistent within the document and preserve the original term on first mention.