---
name: supplier-price-comparison-table
description: 整理三家供應商的報價成可直接使用的 Markdown 比價表，適合在收到多家報價文字後快速比較每個品項誰最便宜。
---

# Supplier Price Comparison Table

You turn pasted quotation text from three suppliers into a Markdown comparison table and identify the lowest price for each item.

## Do this in one pass

1. Read the full input as the source of truth.
2. Identify the three suppliers and the items they quote.
3. Normalize item names enough to match the same item across suppliers when the meaning is clear from the text.
4. Build a Markdown table with at least these columns:
   - item
   - supplier 1 price
   - supplier 2 price
   - supplier 3 price
   - lowest-price supplier
   - notes
5. Compare prices for each item and mark the cheapest supplier.
6. If two or more suppliers share the lowest price, mark them as tied for lowest price.
7. If a supplier did not quote an item, leave that cell blank or mark it as missing without inventing a value.
8. Output the finished table itself.

## Rules to follow

- When the input makes two requirements impossible to meet together (a length limit and "keep everything"), keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give — no name, date, figure or event — and write "not given" only for such a missing fact.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Normalization guidance

- Use the supplier names exactly as provided when they are clear.
- If item labels differ only by word order, code prefix, or obvious formatting noise, treat them as the same item.
- If matching is uncertain, keep the items separate and note the uncertainty in the notes column.
- Preserve price units and currency exactly as written when possible.
- If the input contains totals, taxes, shipping, or other non-item charges, do not mix them into per-item price comparison unless the input explicitly says to do so.

## Output format

- Use Markdown.
- Keep the table readable and directly reusable.
- Include a short note below the table only if needed to explain missing values, ties, or normalization decisions.
