---
name: write-self-introduction
description: When a user needs a self-introduction written from their own details, produce a natural Traditional Chinese introduction tailored to the stated occasion and length.
---

# Write a Self-Introduction

Write a self-introduction from the information the user provides. Make it ready to use immediately, in Traditional Chinese by default, and adapt it to the stated occasion, length, and tone.

## Instructions

1. Read the user's request and extract the usable facts they provided.
2. Identify the intended use case, such as interview, class, social profile, event introduction, or general self-introduction.
3. Write the self-introduction in a natural style that matches the use case.
4. If the user specified a length, keep the output within that length.
5. If the user did not specify a tone, use a neutral, natural tone by default and say that you used the default tone.
6. If any necessary setting is missing, use the common default, state which default you used, and finish the work instead of stopping.
7. Never invent personal facts the user did not provide.
8. If the request contains both a length limit and an instruction to keep everything, obey the hard limit and state in one line what you left out.
9. If the request asks you to send, post, schedule, monitor, or fetch something, provide the content ready to use and say plainly that sending or scheduling is left to the person.
10. Deliver the finished self-introduction itself in the output; do not give a plan or a description of rules.

## Output shape

- Return only the finished self-introduction unless the user asked for brief notes about defaults used.
- When you need to mention defaults or omissions, keep it concise and place it before or after the self-introduction.
- If a fact is missing, write `not given` only for that missing fact and otherwise continue with the best usable draft.

## What to include when the user provides it

- Name or preferred name
- Current role, background, or identity
- Relevant strengths, traits, or experience
- Intended use or audience
- Desired tone, length, or format
- Any key achievements or goals

## What to avoid

- Do not add unsupported dates, numbers, awards, employers, schools, or events.
- Do not ask follow-up questions once enough information is present to write a usable draft.
- Do not explain the writing rules unless the user explicitly asks.

## If the user gives very little information

Use the provided details only and write a short, general self-introduction. Fill gaps with the common default assumptions above and clearly say which defaults were used.