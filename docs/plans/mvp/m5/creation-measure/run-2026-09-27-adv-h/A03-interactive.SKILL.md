---
name: customer-complaint-classifier-summary
description: Summarize a batch of complaint emails into a category count table. Use this when you are given complaint messages and need a concise statistical breakdown by issue type.
---

# Customer complaint classification summary

Create a category summary table from the complaint emails provided in the input.

## Instructions
1. Read only the complaint emails in the input.
2. Group complaints by issue type using sensible category names based on the email content.
3. Count how many emails belong to each category.
4. Output a table with at least these columns:
   - category
   - count
   - representative issue summary
5. If multiple emails describe the same or similar issue, place them in the same category.
6. Use only facts present in the input. Never invent details not given.
7. If a necessary setting is missing, use the common default, say which one you used in the output, and finish the work.
8. If the input makes two requirements impossible to satisfy together, keep the hard limit and say in one line what you left out; never drop it silently.
9. You cannot send, post, schedule, monitor, or fetch anything. If the request asks for that, produce the content ready to use and state plainly that sending or scheduling is left to the person.
10. Deliver the finished artifact itself, not a plan or a description of rules.

## Output format
- Start with a short title.
- Then provide a markdown table.
- Then add a brief note if any assumption was needed.

## Defaults
- If no category labels are provided, derive labels from the complaint themes in the input.
- If the input contains multiple complaint topics, include one row per topic.
- If the input is already grouped or labeled, preserve those labels unless they conflict with the content.

## Quality check
Before finishing, verify that every complaint is counted exactly once and that the totals match the number of complaint emails in the input.