---
name: competitor-price-change-tracker
description: Track price changes for three competitor websites and produce a change summary when prices differ from the previous round. Use it when you need a reusable comparison step before sending a notification or updating a monitoring workflow.
---

# Competitor Price Change Tracker

Use this Skill when you need to compare prices for three competitor websites against a previous known set of prices and produce a clear change summary.

## What this Skill does

1. Read the current prices and the previous prices from the input.
2. Compare the three competitors using the same comparison target.
3. Identify every item whose price changed.
4. Identify every item whose price stayed the same.
5. Output a compact summary that can be used by a downstream notification system.
6. If no prices changed, say so explicitly.

## Assumptions

- The input already contains the relevant prices or enough structured text to extract them.
- The Skill does not fetch webpages.
- The Skill does not send notifications.
- The Skill compares three competitors against one shared comparison target.
- If a price format is inconsistent, keep the original price string and mark the inconsistency.

## Procedure

### 1) Read the input

Extract, for each competitor:
- competitor name
- current price
- previous price
- the product, plan, or item being compared

If the input does not name one of these facts, write `not given` for that missing fact.

### 2) Compare current and previous prices

For each competitor:
- If the price changed, record the old price, the new price, and the direction of change.
- If the price did not change, record that it is unchanged.
- If the price format differs between current and previous values, keep both original strings and note that the format is inconsistent.

### 3) Build the output

Return the finished artifact itself in the output. Do not describe the rules, provide a plan, or ask for access.

Use this structure:
- Title line naming the comparison
- A `Changed` section listing each changed competitor
- An `Unchanged` section listing each unchanged competitor
- A short concluding line stating whether any change was found

If there are no changes, the output must still explicitly say that there were no price changes.

## Required instructions

when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently; never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output style

- Be concise.
- Keep raw price strings when format differences exist.
- Make the result directly usable as a summary for later notification steps.