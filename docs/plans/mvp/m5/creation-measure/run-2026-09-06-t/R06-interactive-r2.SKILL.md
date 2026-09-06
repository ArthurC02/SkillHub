---
name: english-manual-traditional-chinese-translator
description: Translate English user manuals into Traditional Chinese, preserving the original term in parentheses the first time each proper noun or technical term appears. Use this skill when you need a faithful manual translation with consistent terminology.
---

# English Manual to Traditional Chinese Translator

Translate the provided English user manual into Traditional Chinese.

## What this skill does

Translate English user manual text into clear Traditional Chinese while preserving meaning, terminology, and manual-style readability.

## Core rules

- Translate faithfully into Traditional Chinese.
- Keep terminology consistent throughout the document.
- When a proper noun or technical term appears for the first time, render it as `中文（Original Term）`.
- When the same term appears again later in the same document, use only the Chinese rendering.
- Preserve the original meaning and practical instructions without adding explanations, summaries, or new information.
- Keep the output readable as a manual, including paragraphs, lists, labels, and warnings when present in the source.

## How to work

1. Read the full manual text before translating.
2. Identify proper nouns and technical terms that should be preserved on first mention.
3. Choose one consistent Chinese rendering for each term and reuse it throughout the document.
4. Translate the entire manual into Traditional Chinese.
5. Format first mentions as `中文（Original Term）`.
6. Omit the original term on later mentions of the same item.
7. Do not add content that is not supported by the source text.

## Output

Return only the translated manual text.

## Example behavior

- First mention: `主開關（Main Switch）`
- Later mention: `主開關`
- Source text remains the only basis for the translation; do not expand or reinterpret it.