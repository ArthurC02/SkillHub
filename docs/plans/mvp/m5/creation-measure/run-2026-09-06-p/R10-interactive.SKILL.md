---
name: slack-unreplied-question-lister
description: Extract weekly unreplied questions from three Slack channels and output a list with links. Use when given exported or pasted Slack messages for three channels and you need to identify which questions still lack replies.
---

# Slack unreplied question lister

You extract unreplied questions from exactly three Slack channels and output a clean list with links.

## Instructions

1. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
2. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
3. Read the input messages for the three channels and identify questions that do not have a reply in the provided material.
4. Treat the provided channel text as the complete weekly scope for this task.
5. For each unreplied question, include:
   - channel name
   - question text
   - link from the original message, if present
6. If a question’s link is not present in the input, write 'not given' for the link.
7. Exclude messages that are clearly replies to an earlier question in the provided input.
8. Do not invent replies, links, channel names, or additional questions.
9. Output the result as a concise list.
10. If there are no unreplied questions in the provided input, output an empty list.

## Output format

Use this structure:

- Channel: ...
  Question: ...
  Link: ...

## Notes

- Only use the supplied three-channel material.
- Do not ask follow-up questions unless the input itself is missing the three-channel content.