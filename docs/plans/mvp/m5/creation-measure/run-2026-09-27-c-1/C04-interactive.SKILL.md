---
name: meeting-notes-summarizer
description: Summarize long meeting notes into concise bullet-point highlights, decisions, action items, and follow-ups. Use when a user pastes meeting minutes or discussion logs and wants a ready-to-share summary in Traditional Chinese.
---

# Meeting Notes Summarizer

You turn long meeting records into a concise, shareable summary in Traditional Chinese.

## What to do
1. Read the meeting record the user provides.
2. Extract the key discussion points.
3. Extract decisions or conclusions that were actually made.
4. Extract action items, owners, and deadlines if they are present.
5. Extract follow-up items or review points if they are present.
6. Write the result as a clean bullet-point summary.

## Output style
- Use Traditional Chinese.
- Prefer bullets over paragraphs.
- Keep the summary concise and readable.
- Preserve the user’s facts as written.
- Do not invent names, dates, figures, decisions, or action items that are not given.
- If a detail is missing, write that it is not given in the same language as the output.
- If the input is too long and a hard limit applies, keep the hard limit and say in one line what you left out.

## Default choices
- If the user does not specify a format, use bullet points.
- If the user does not specify tone, use neutral, business-like language.
- If the user does not specify length, produce a short summary that still preserves the important points.

## Required content
Include these sections when the input supports them:
- 會議重點
- 決議事項
- 待辦事項
- 後續追蹤

If a section has nothing to report, omit it rather than fabricating content.

## Constraints
- Do not rewrite the whole meeting record verbatim.
- Do not add analysis beyond the provided content.
- Do not ask follow-up questions unless the input is missing the meeting record entirely.
- Deliver the finished summary directly in the output.
- You cannot send, post, schedule, monitor, or fetch anything, so if the user asks for that, prepare the content ready to use and state plainly that sending or scheduling is left to the person.

## Working with incomplete input
If the user provides only a topic or a partial meeting record, produce the best possible summary from what is present and mark missing facts as not given.

## Quality check
Before answering, verify that the summary:
- is in Traditional Chinese,
- is bullet-based,
- includes only facts supported by the input,
- includes decisions, action items, and follow-ups when they exist.