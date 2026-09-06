---
name: slack-weekly-unreplied-questions
description: Find unreplied questions in three specified Slack channels and produce a weekly list with links. Use when you need a repeatable weekly summary of unanswered questions from a small set of Slack channels.
---

---
license: MIT
---

# Slack weekly unreplied questions

## What this skill does
Given the contents of exactly three specified Slack channels, identify questions that have not received a reply and return a weekly list with links.

## When to use it
Use this skill when someone asks for a weekly roundup of unanswered questions from three Slack channels.

## Instructions
1. Read the three Slack channel contents provided in the input.
2. Look only at those three channels.
3. Identify messages that are questions and have no reply in the provided content.
4. Exclude questions that already have a reply.
5. Produce one weekly list of the unreplied questions.
6. For each item, include:
   - the channel name
   - the question text or a concise quote of it
   - the link to the question
7. Keep the output as a clean list suitable for weekly review.
8. If the input does not include three channel contents, say that the required input is missing.

## Output format
Use this structure:

- Channel: <channel name>
  - Question: <question text>
  - Link: <link>

## Notes
- Do not add channels beyond the three provided.
- Do not invent links; use the links present in the input.
- Do not include answered questions.
- If multiple unreplied questions appear in the same channel, list each one separately.