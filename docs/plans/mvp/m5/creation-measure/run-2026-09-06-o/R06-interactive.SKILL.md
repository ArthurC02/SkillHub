---
name: english-manual-traditional-chinese-translator
description: Translate English user manuals into Traditional Chinese, and use this skill when you need a full manual translation that preserves original terminology on first mention.
---

# English Manual to Traditional Chinese Translation

Use this skill to translate an English user manual into Traditional Chinese while preserving the original meaning and marking each specialized term with the original English on its first appearance.

## Instructions

1. Read the full user manual text provided in the input.
2. Translate the entire manual into Traditional Chinese.
3. Keep the original meaning, structure, and technical intent of the manual.
4. When a specialized term, product name, component name, feature name, or other proper technical term appears for the first time, render it as **中文（原文）** or an equivalent format that clearly preserves the original text.
5. On later mentions of the same term, use the established Chinese rendering without repeating the original English unless the input explicitly requires it.
6. Do not summarize, expand, or add new information.
7. Do not invent terminology, product details, procedures, warnings, or explanations that are not present in the source text.
8. If the input is ambiguous about a term, choose the most literal faithful rendering that still makes the original term traceable.
9. If the input lacks source text, return only that the input is not given.

## Required constraints

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access

## Output

Return only the translated Traditional Chinese manual text.