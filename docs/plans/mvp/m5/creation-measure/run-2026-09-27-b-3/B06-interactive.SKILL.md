---
name: supplier-quote-comparator
description: 整理三家供應商的文字報價成比價表，適合在需要逐品項找出最低價與並列最低價時使用。
---

# Supplier Quote Comparator

Use this Skill when a user pastes quote text from three suppliers and asks for a comparison table that shows which supplier is cheapest for each item.

## What to do

1. Read the pasted quote text as the only source of truth.
2. Identify each supplier, each item name, and each unit price.
3. Build one comparison table that keeps the original supplier names, item names, and prices.
4. Compare prices item by item, not by total quote.
5. Mark the cheapest supplier for each item.
6. If two or more suppliers share the same lowest price for an item, mark all of them as tied lowest.
7. Keep the original currency and unit exactly as given; do not convert them.
8. If the input is incomplete or ambiguous, use the common default interpretation that best fits the pasted text, state that assumption in the output, and still finish the table.
9. Deliver the finished comparison table itself in the output.

## Output format

Return a clean comparison table with columns such as:
- Item name
- Supplier A price
- Supplier B price
- Supplier C price
- Lowest price
- Cheapest supplier
- Notes for ties or assumptions

If the input uses more than one way to write the same item name, group only when the equivalence is clear from the text.

## Rules to follow

- when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently; never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Notes

- The user may provide all suppliers in one block or in separate blocks.
- Do not add external facts.
- Do not change currencies or units.
- If the text clearly names a tie, show the tie instead of choosing one supplier.
