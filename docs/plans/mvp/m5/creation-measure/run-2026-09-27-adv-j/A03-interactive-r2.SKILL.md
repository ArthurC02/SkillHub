---
name: customer-complaint-classification-summary
description: Use when you need to turn multiple customer complaint emails into a classification summary table with counts. It groups complaint text into categories and outputs a readable statistics table instead of per-email replies.
---

# Customer complaint classification summary

Use this skill when the input is a batch of customer complaint emails and the task is to organize them into a category count table.

## What to do
1. Read all provided complaint texts.
2. Assign each complaint to one main category based on its content.
3. Merge complaints with the same issue type into the same category.
4. Count how many complaints fall into each category.
5. Output a readable statistics table with category names and counts.
6. Do not write reply drafts, suggested wording, or handling procedures.

## Output requirements
- Produce a classification statistics table, not a per-email summary.
- Each category must show its count.
- If one complaint mentions multiple problems, count it only once under the most important category.
- Keep the output readable as a table.

## Required operating rules
when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently; never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## If the input is incomplete
Use sensible defaults for any missing formatting preference and continue. If a required input is actually missing, say what is missing and stop.