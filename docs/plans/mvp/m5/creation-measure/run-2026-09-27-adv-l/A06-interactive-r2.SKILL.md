---
name: self-introduction-writer
description: Writes a natural Chinese self-introduction for general use, interview use, or submission-ready short bios when the user provides some personal details or asks for a generic version.
---

# Self-Introduction Writer

Write a Chinese self-introduction that the user can use directly. Choose a sensible default style when the user does not specify one: use a polished, general short-form written introduction suitable for school, work, or everyday submission. State that default in the output.

## What to do

1. Read the user’s request and any personal details they provide.
2. If the user specifies an audience, length, tone, or use case, follow it.
3. If those settings are missing, use the common default: a general written short-form self-introduction with a neutral, friendly tone and moderate length. State which default you used.
4. Include the core elements that fit the input: name or preferred称呼, background, skills or traits, experience or interests, and a closing line.
5. If some personal facts are missing, do not invent them. Use `not given` only for facts that are truly absent.
6. If the request needs a specific format, produce that format. If the format is missing, use the format that best fits the request and say which one you used.
7. If the user specifies a use case such as interview, resume, or social opening, adjust the tone and length to fit that use case while keeping the same core self-introduction content.
8. Deliver the finished self-introduction itself in the output.

## Required operating rules

when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently; never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output guidance

- Write in clear, natural Chinese.
- Keep the wording ready to paste and use.
- Do not add explanations unless the user asks for them.
- If the user asks for multiple versions, provide them directly.
- If the input is only a topic and no personal facts, write a usable generic version with clearly marked placeholders such as `【姓名】`, `【背景】`, `【經歷】`, and `【興趣】`.

## If the user asks for a different use case

- Interview: use a more persuasive, motivation-focused tone.
- Resume: use a concise, professional bullet or short-paragraph style.
- Social opening: use warmer, more conversational phrasing.
- School or submission use: keep it balanced and formal.

Keep the final output as the rewritten self-introduction, not advice about it.

## Final check

Before answering, make sure the text is coherent, complete, and directly usable. Do not ask follow-up questions once a default can be applied.