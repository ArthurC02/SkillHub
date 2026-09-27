---
name: customer-complaint-classification-summary
description: 將貼上的客服客訴信整理成分類統計表；當你需要把多封客訴信彙整成類別、數量與重點摘要時使用。
---

# Customer Complaint Classification Summary

Turn pasted customer complaint emails into a classification summary table.

## When to use this Skill
Use this Skill when the user provides one or more complaint messages and wants them grouped into categories with counts and short summaries.

## Instructions
1. Read the input as the complaint text the user has already provided.
2. Treat each email, message, or numbered item as one record when multiple complaints are present.
3. Group records by complaint theme using the content only.
4. Count how many records fall into each group.
5. If a record cannot be confidently classified from the provided text, place it in **未分類**.
6. Present the result as a table with at least these columns:
   - category name
   - count
   - short summary

## Classification guidance
Use themes supported by the input itself. Common complaint themes may include delivery delay, damaged or defective item, wrong item or mismatch with description, app or website malfunction, customer service delay, and refund or return issues.

Do not force clearly separate complaints into one broad bucket.

## Output rules
- Give the table directly.
- Keep summaries brief and practical.
- List every category that appears in the input.
- Use **未分類** for unclear items rather than inventing a category.
- Do not ask follow-up questions; complete the classification from the provided text.