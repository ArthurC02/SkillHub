---
name: supplier-price-comparison-table
description: 整理三家供應商貼上來的報價成可直接使用的 Markdown 比價表，並標出每個品項的最低價與是否並列最低價。
---

# Supplier Price Comparison Table

You turn pasted quotation text from three suppliers into a Markdown comparison table and identify the lowest price for each item.

## What to produce

Return the finished comparison table itself. Do not stop at an acknowledgement, a plan, or a request for more input.

## Workflow

1. Read the entire input as the source of truth.
2. Identify the three suppliers and their quoted items.
3. Normalize item names enough to match the same item across suppliers when the meaning is clear from the text.
4. Build a Markdown table with at least these columns:
   - item
   - supplier 1 price
   - supplier 2 price
   - supplier 3 price
   - lowest-price supplier
   - notes
5. Compare prices for each item and mark the cheapest supplier.
6. If two or more suppliers share the lowest price, mark them as tied for lowest price in the notes or lowest-price column.
7. If a supplier did not quote an item, leave that cell blank or mark it as missing without inventing a value.
8. Include a short note below the table only when needed to explain missing values, ties, or normalization decisions.

## Normalization guidance

- Use the supplier names exactly as provided when they are clear.
- If item labels differ only by word order, code prefix, or obvious formatting noise, treat them as the same item.
- If matching is uncertain, keep the items separate and note the uncertainty in the notes column.
- Preserve price units and currency exactly as written when possible.
- If the input contains totals, taxes, shipping, or other non-item charges, do not mix them into per-item price comparison unless the input explicitly says to do so.

## Output rules

- Use Markdown.
- Keep the table readable and directly reusable.
- Do not invent missing prices or unnamed suppliers.
- Do not convert the result into prose when a table is possible.
