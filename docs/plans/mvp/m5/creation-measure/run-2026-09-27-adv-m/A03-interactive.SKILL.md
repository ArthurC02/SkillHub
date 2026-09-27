---
name: customer-complaint-category-summary
description: 將客服信箱中的客訴信依主題整理成分類統計表；當你需要把一批投訴信快速彙整成可讀的分類與件數時使用。
---

# Customer Complaint Category Summary

You will receive a batch of customer complaint emails or similar complaint text. Your job is to turn the provided material into a category summary table.

## Required behavior

1. Read only the input you are given.
2. Identify the complaint topic for each item using the evidence available in the text, such as subject, body, date, or labels if present.
3. Group items into a sensible category table and count how many items fall into each category.
4. If one item mentions multiple problems, assign one main category using the most central complaint in the text.
5. If some items cannot be classified clearly, place them in `其他`.
6. Output the finished artifact itself: a classification summary table with counts, ready to use.

## Operating rules

- When the input makes two requirements impossible to meet together, keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give — no name, date, figure or event — and write `not given` only for such a missing fact.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output format

Use a simple table with these columns:

- `分類`
- `信件數`
- `判定依據`

If the user provided enough detail to make a more precise breakdown, include it in the `判定依據` column briefly. If not, keep the reasoning concise and based only on the given text.

## Suggested category labels

Use common complaint categories when they fit the input, such as:

- `物流/配送`
- `商品品質`
- `客服態度`
- `退款/退貨`
- `付款/帳務`
- `系統/網站`
- `其他`

Do not force these labels if the input clearly supports a different label.

## Handling ambiguous or mixed complaints

- Prefer the category that best matches the main complaint.
- If two issues are equally central and the input does not provide a rule, choose the category that is most directly actionable from the wording.
- Mention the choice briefly in `判定依據` when it may not be obvious.

## If the input is incomplete

If the user gives only a request without actual complaint text, produce a usable template and say what must be filled in. If the user gives complaint text, do the classification directly in one pass.