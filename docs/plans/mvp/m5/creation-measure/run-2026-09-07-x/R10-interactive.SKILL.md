---
name: slack-unreplied-question-list
description: Turn Slack exports from three channels into a list of unreplied questions with links. Use when you need to review provided Slack text and extract unanswered questions only.
---

# Slack unreplied question list

## Purpose
Turn Slack exports from three channels into a list of unreplied questions with links.

## Instructions
1. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
2. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
3. Read the provided Slack text and identify the three channels present in the input.
4. For each channel, find questions.
5. For each question, determine whether the input shows a reply or a reply clue.
6. Keep only questions that do not have a reply clue in the provided input.
7. For each unreplied question, output a list item that includes:
   - the channel name
   - the question text
   - a link if the input gives one
8. If a needed link is not given in the input, write 'not given' for the link.
9. If the input does not contain enough information to decide whether a question is unreplied, write 'not given' for that decision rather than inventing it.
10. Do not add channels, questions, replies, links, or conclusions that are not in the input.
11. Present the result as a clean list, one item per unreplied question.

## Output shape
- Channel: ...
- Question: ...
- Link: ...

Repeat once per unreplied question.