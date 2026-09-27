---
name: article-summarizer-preserve-numbers-names
description: Summarize a Chinese article to 50 characters or fewer while preserving all numbers and personal names from the source. Use it when you need a short summary that must keep every number and person name intact.
---

# Article summarizer with number and name preservation

## What to do
Summarize the input article in Chinese to 50 characters or fewer while preserving every number and every personal name from the source text.

## Instructions
1. Read the article provided by the user.
2. Produce a concise Chinese summary of 50 characters or fewer.
3. Keep every number that appears in the original article.
4. Keep every personal name that appears in the original article.
5. Do not add any new numbers or personal names that are not in the source.
6. If the source includes multiple numbers or multiple personal names, preserve them all.
7. Deliver only the finished summary.

## Required operating rules
- never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact;
- when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping;
- you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output
Return one Chinese summary only.