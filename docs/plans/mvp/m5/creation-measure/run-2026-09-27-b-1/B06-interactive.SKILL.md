---
name: supplier-quote-comparison-table
description: Build a comparison table from three suppliers’ quotes and identify the cheapest supplier for each item. Use this when a user provides quotation lines or a list of supplier prices and needs a direct buying comparison.
---

# Purpose
Turn supplier quotes into a comparison table and identify the cheapest supplier for each item.

# What to do
1. Read the input quotes exactly as given.
2. Group quotes by item name.
3. Compare the prices for each item across the suppliers.
4. Produce a comparison table that includes:
   - the item name,
   - each supplier’s price,
   - the lowest price,
   - the cheapest supplier, or all tied cheapest suppliers when there is a tie.
5. Preserve the exact item names and supplier names from the input.
6. If the same item appears more than once, combine those rows before comparing.
7. If the input contains ties for the lowest price, list every tied supplier.
8. If the input is incomplete, use the common default where appropriate, state which default you used, and finish the table instead of stopping.

# Required behavior
- Never invent a supplier name, item name, price, date, figure, or event that the input does not give.
- Write `not given` only for a missing fact from the input.
- If the request also asks for something that cannot be done together with a hard limit, keep the hard limit and say in one line what was left out.
- When the work involves sending, posting, scheduling, monitoring, or fetching, prepare the content ready to use and say clearly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output; do not describe the rules or ask follow-up questions.

# Output format
Return a clean comparison table in Markdown.

Suggested columns:
- Item
- Supplier 1
- Supplier 2
- Supplier 3
- Cheapest supplier
- Lowest price
- Notes for ties or grouping

If the input uses different supplier names or more/less than three suppliers, keep the same structure logically and match the names actually provided.

# Quality check
Before finishing, verify that every item from the input appears in the table and that each item has a cheapest supplier determination.