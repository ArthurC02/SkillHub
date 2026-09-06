---
name: user-manual-traditional-translation
description: Translate an English user manual into Traditional Chinese, preserving the original English the first time each proper noun or defined product term appears. Use when the input is a manual or manual excerpt that must stay structurally faithful in translation.
---

# User Manual Translation Skill

You translate an English user manual into Traditional Chinese.

## When to use this skill
Use this skill when the user provides an English manual or manual excerpt and wants a complete Traditional Chinese translation, with proper nouns or technical terms shown in English the first time they appear.

## What to do
1. Read the full input text once before translating so you can keep terminology consistent.
2. Translate all user-facing content into Traditional Chinese.
3. Preserve the original English the first time each proper noun or clearly named product/component term appears by writing it as `繁中譯名（Original English）` or `Original English（繁中譯名）` if the source term is already widely recognized in English-first form.
4. After the first mention, use the chosen Traditional Chinese term consistently without repeating the English unless the source text itself repeats a formal label that must remain visible.
5. Keep the meaning, section order, warnings, notes, steps, and formatting structure as close to the source as possible.
6. Preserve tables, numbered lists, bullet lists, labels, callouts, and warning blocks in the translated output rather than rewriting them into prose.
7. Do not add new instructions, explanations, summaries, or omissions.
8. If a term is ambiguous, choose the most literal stable translation that fits the manual context and keep it consistent throughout the document.
9. If the source already defines a preferred translation for a term, reuse that translation consistently.
10. Output only the translated manual; do not add commentary or a separate explanation.
11. Do not invent missing sections or rewrite the manual into a different format.
12. If the provided input is incomplete, translate only the provided material and do not guess missing content.

## Output rules
- Output only the translated manual unless the user explicitly requests commentary.
- Use Traditional Chinese throughout.
- Do not repeat original English on every occurrence; only the first occurrence should include it.
- Keep document structure intact, including headings, lists, tables, and notes.
- Do not invent missing sections or reformat the manual into a different genre.

## Style
- Use clear, neutral Traditional Chinese suitable for documentation.
- Prefer terminology consistency over stylistic variation.
- Keep technical labels concise and readable.