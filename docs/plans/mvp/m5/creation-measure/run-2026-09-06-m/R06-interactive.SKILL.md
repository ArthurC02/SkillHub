---
name: english-manual-to-traditional-chinese
description: Translate an English user manual into Traditional Chinese. Use this when the user provides a manual and wants a faithful translation with first-use original terms shown.
---

# English Manual to Traditional Chinese

You translate the English user manual you are given into Traditional Chinese.

Follow these rules:

- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- Preserve the original meaning and keep terminology consistent throughout.
- Translate the entire manual that is provided.
- When a technical term or other proper specialized term appears for the first time, show the original once in the form `中文（Original）` or an equivalent clear first-use notation.
- After the first mention, use the same Traditional Chinese term consistently.
- Keep the existing structure of the manual when it is present: headings, numbering, bullets, warnings, and ordering should be preserved unless the source format makes that impossible.
- Do not add explanations, summaries, examples, or product details that are not in the source.
- If the source text is incomplete or a section is missing, translate only the text provided and mark the missing part as `not given`.

## Output behavior

Produce only the translated manual content in Traditional Chinese, unless the input itself requires a brief note to preserve clarity about missing text.

## Translation procedure

1. Read the full manual.
2. Translate each section in order.
3. Keep terminology stable across the document.
4. On first occurrence of each specialized term, include the original term once.
5. Check that nothing new was introduced.

## Quality check

Before finalizing, verify that:

- the output is entirely in Traditional Chinese,
- all provided content has been translated,
- first-use original terms are shown,
- no unsupported information was added,
- terminology is consistent.