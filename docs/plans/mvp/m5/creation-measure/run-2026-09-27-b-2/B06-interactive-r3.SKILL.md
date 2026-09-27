---
name: supplier-price-comparison-table
description: 整理三家供應商貼上來的報價成可直接使用的 Markdown 比價表，並標出每個品項的最低價與是否並列最低價。
---

# Supplier Price Comparison Table

Produce a Markdown comparison table from pasted quotation text from three suppliers, and return the table itself in your first response.

## What to produce

Return the finished comparison table itself. Do not stop at an acknowledgement, a plan, or a request for more input. Do not ask the user to continue; perform the comparison immediately on the provided text.

For every item, include the three supplier prices in the same row and identify which supplier is lowest for that item.

## Workflow

1. Read the entire input as the source of truth.
2. Identify the three suppliers and their quoted items.
3. Normalize item names enough to match the same item across suppliers when the meaning is clear from the text.
4. Build a Markdown table with these columns:
   - item
   - supplier 1 price
   - supplier 2 price
   - supplier 3 price
   - lowest-price supplier
   - notes
5. Compare prices for each item and mark the cheapest supplier.
6. If two or more suppliers share the lowest price, explicitly mark all tied suppliers as tied for lowest price in the lowest-price supplier column or notes column.
7. If a supplier did not quote an item, keep that supplier’s cell empty or mark it as missing in the table, and never invent a price.
8. Include a short note below the table only when needed to explain missing values, ties, or normalization decisions.

## Normalization guidance

- Use the supplier names exactly as provided when they are clear.
- If item labels differ only by word order, code prefix, or obvious formatting noise, merge them into the same row when they clearly refer to the same item.
- If there is any reasonable match, prefer one row over splitting the same item into multiple rows.
- If matching is uncertain, keep the items separate and note the uncertainty in the notes column.
- Preserve price units and currency exactly as written when possible.
- If the input contains totals, taxes, shipping, or other non-item charges, do not mix them into per-item price comparison unless the input explicitly says to do so.

## Output rules

- Use Markdown.
- Keep the table readable and directly reusable.
- Do not invent missing prices or unnamed suppliers.
- Do not convert the result into prose when a table is possible.
- Do not leave a tie ambiguous; the output must make the tie visible in the table.
- Missing prices must remain visibly blank or marked as missing in the row.