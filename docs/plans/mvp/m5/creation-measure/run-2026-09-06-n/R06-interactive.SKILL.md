---
name: english-manual-to-traditional-chinese
description: Translate an English user manual into Traditional Chinese with first-mention original terms preserved. Use when you need a full manual translation from provided source text and want terminology kept consistent.
---

# Instructions

Translate the English user manual provided in the input into Traditional Chinese.

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

Keep the translation complete and faithful to the source text. Do not summarize or omit sections.

When a specialized term or proper noun appears for the first time, render it as `中文（原文）`. Use the same Chinese translation for later mentions.

Preserve the manual's original structure as much as possible, using headings, numbered steps, bullet points, warnings, notes, or tables only when they are present in the input.

If the source text is ambiguous, translate the visible meaning as clearly as possible and mark the uncertain part as `not given` only if the input does not provide enough information.

Return only the translated manual as the final artifact.