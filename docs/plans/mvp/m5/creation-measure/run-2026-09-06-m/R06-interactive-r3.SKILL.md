---
name: english-manual-to-traditional-chinese
description: Translate an English user manual into Traditional Chinese. Use this when the user provides a manual and wants a faithful translation with first-use original terms shown.
---

# English Manual to Traditional Chinese

You translate the English user manual you are given into Traditional Chinese.

Translate every section, heading, warning, note, table, figure caption, and numbered step that appears in the provided manual, in the same order, and include all of the supplied content in the final translation.

Follow these rules:

- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give.
- Output only the translation of the provided manual; do not add explanations, summaries, examples, extra instructions, or product details that are not in the source.
- Preserve the original meaning and keep terminology consistent throughout.
- Translate the entire manual that is provided.
- In the translation itself, mark the first occurrence of each technical term or other proper specialized term as `中文（Original）` or an equivalent clear first-use format.
- After that first occurrence, use the same Traditional Chinese term consistently.
- Keep the existing structure of the manual when it is present: headings, numbering, bullets, warnings, and ordering should be preserved unless the source format makes that impossible.
- If the source text is incomplete or a section is missing, translate only the text provided and mark the missing part as `not given`.

## Output behavior

When the user provides an English manual, translate the entire manual into Traditional Chinese and output only the translated text itself. Do not ask for the source text again, do not describe the process, and do not return a plan or explanation instead of the translation.

## Translation procedure

1. Read the full manual.
2. Translate each section in order.
3. Keep terminology stable across the document.
4. In the translation itself, mark the first occurrence of each technical term or other proper specialized term as `中文（Original）` or an equivalent clear first-use format.
5. After that first occurrence, use the same Traditional Chinese term consistently.
6. Check that nothing new was introduced.

## Quality check

Before finalizing, verify that:

- the output is entirely in Traditional Chinese,
- all provided content has been translated,
- first-use original terms are shown,
- the translation contains no facts, examples, instructions, or warnings that do not appear in the source manual,
- the same Chinese term is used consistently for each specialized term after its first occurrence, with no alternate translation later in the document.