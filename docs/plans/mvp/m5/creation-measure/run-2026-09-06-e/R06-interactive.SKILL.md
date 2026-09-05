---
name: manual-translation-zh-tw
description: Translate an English user manual into Traditional Chinese while preserving the manual’s structure and adding the original term in parentheses the first time each specialized term appears. Use when you need a faithful zh-TW manual translation with terminology tracking.
---

# Purpose
Translate an English user manual into Traditional Chinese (Taiwan) while preserving the manual’s structure and adding the original English term the first time each specialized term appears.

# When to use
Use this Skill when the input is a user manual, guide, or handbook in English and the required output is a faithful Traditional Chinese translation with terminology handled consistently.

# Workflow
1. Read the entire manual before translating so terminology and structure stay consistent.
2. Preserve the original organization as much as possible, including headings, numbered steps, lists, tables, warnings, notes, and labels.
3. Translate into natural Traditional Chinese that matches instructional/manual tone.
4. Track specialized terms, product names, UI labels, feature names, and other terms that should be kept with their original wording on first mention.
5. On the first occurrence of each tracked term, write the Traditional Chinese translation and add the original English term in parentheses immediately after it.
6. On later occurrences, use only the chosen Traditional Chinese term unless including the original term is needed to avoid ambiguity.
7. Do not invent explanations, cautions, steps, or examples that are not present in the source.
8. If the source contains unclear or inconsistent terminology, translate conservatively and keep the wording close to the source.

# Translation rules
- Preserve meaning over literal word order.
- Keep section order, paragraph breaks, list nesting, and table structure intact unless the source itself changes the structure.
- Do not summarize or omit content unless the user explicitly asks for abbreviation.
- Keep brand names, model names, file names, menu paths, button labels, and code snippets unchanged unless a clear Chinese rendering is needed for the manual style.
- If a term is already a widely recognized proper noun or product name, leave it in the original form and do not force a translation.
- If a term appears multiple times within the same immediately repeated heading or list item, apply the first-mention rule consistently within the translated document.
- If the manual contains placeholders, form fields, or placeholders in brackets, preserve them exactly.

# Output format
- Output only the translated manual.
- Keep the same visible structure as the source whenever possible.
- Do not add a preface, commentary, translation notes, or a glossary unless the user explicitly requests them.

# Quality checks before finalizing
- Confirm every major section in the source has a corresponding translated section.
- Confirm first-mention terminology includes the original English term in parentheses.
- Confirm repeated terminology does not unnecessarily repeat the original term.
- Confirm no new instructions or warnings were introduced.