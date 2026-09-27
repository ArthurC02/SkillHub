---
name: self-introduction-writer
description: Write a first-person self-introduction from the background details the user provides. Use it when someone needs a ready-to-use self-intro for a general, interview, resume, or social profile context and may have missing details.
---

# Self-Introduction Writer

Write a first-person self-introduction from the information the user gives you.

## What this skill does
- Turns background details into a polished self-introduction.
- Works for general introductions, interview introductions, resume-style summaries, or social-profile intros.
- Uses the language of the user’s input.

## How to handle the request
1. Identify the user’s requested use case, tone, and length if they are present.
2. Extract the facts the user actually provided, such as name, role, education, experience, interests, and strengths.
3. Draft the self-introduction in first person.
4. Keep the output natural, clear, and ready to use.
5. If important settings are missing, use the common default and state which default you used in the finished output.

## Defaults
- If the purpose is missing, use a general-purpose self-introduction.
- If the tone is missing, use a natural, slightly formal tone.
- If the length is missing, use about one to two paragraphs.
- If the language is missing, match the dominant language of the user’s message.

## Required constraints
- Never invent personal facts, names, dates, figures, events, or achievements that the user did not provide.
- When a fact is missing, write it as "not given" only if you need to mention the missing fact explicitly.
- If the request asks for both a strict length limit and to include everything, keep the hard limit and state in one line what you left out.
- When a setting the work needs is missing, use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor, or fetch anything; if the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output, not a description of the rules, a plan, or a request for access.

## Writing approach
- Prefer concise paragraphs over bullet points unless the user asks for bullets.
- Connect the user’s facts into a smooth narrative.
- Emphasize the details that fit the requested purpose.
- Do not add claims beyond the user’s material.

## Output
Return only the self-introduction text unless the user explicitly asks for an explanation or multiple versions.