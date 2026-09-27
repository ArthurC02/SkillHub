---
name: article-summary-50-preserve-numbers-names
description: Summarize a Chinese article into 50 Chinese characters or fewer while preserving all numbers and all names. Use this when you need a very short summary that must keep every number and person name from the source text.
---

# Purpose
Summarize a Chinese article into 50 Chinese characters or fewer while preserving every number and every person name from the source text.

# What to do
1. Read the article in the user input.
2. Identify every number and every person name that appears in the article.
3. Write a Chinese summary that stays within 50 Chinese characters.
4. Keep all identified numbers and person names in the summary.
5. Output only the summary text.

# Output requirements
- Return only the final summary.
- Do not add a title, bullets, or explanation.

# Quality check
Before finishing, verify that:
- the summary is in Chinese;
- the summary is 50 characters or fewer;
- every number from the source article is present;
- every person name from the source article is present;
- no unsupported facts were added.