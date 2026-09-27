---
name: complaint-email-category-summary
description: Sort pasted customer complaint emails into a category summary table with counts and short issue summaries. Use this when the user provides complaint email text and wants themes grouped for reporting.
---

# Goal
Turn pasted customer complaint email text into a category summary table.

# What to do
1. Read the full user-provided text.
2. Identify each separate complaint email or complaint entry in the text.
3. Assign each complaint to one main category.
   - If one complaint mentions several issues, choose the most central issue as the main category.
   - If a category label is not obvious, use a plain, understandable label.
4. Count how many complaints fall into each category.
5. Write a summary table with:
   - category name
   - count
   - short summary of the main complaint in that category
6. Add a brief overall summary if useful.

# Output requirements
- Output the finished artifact itself, not a plan.
- Use a clear table.
- Keep category names consistent across the table.
- Make the counts add up to the number of complaints included in the analysis.
- If the input mixes multiple topics in one complaint, do not split it into multiple rows unless the user explicitly asked for that.

# Handling missing or unclear information
- Never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- When the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.

# Assumptions for this Skill
- Default language follows the user's input.
- Default format is a concise markdown table.
- Default scope is only the complaint text pasted in the message.
- Default grouping rule is one main category per complaint.

# Suggested table shape
| Category | Count | Main issue summary |
|---|---:|---|
| ... | ... | ... |

# Final check
Before finishing, verify that every complaint has been counted exactly once and that the totals are internally consistent.