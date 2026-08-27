---
name: dedupe-mailing-list-spreadsheet
description: Use when a mailing list spreadsheet contains duplicate people from merged exports and you need one row per person without changing the formatting of the rows that remain.
---

# Deduplicate a mailing list spreadsheet without changing surviving row formatting

Use this skill when a spreadsheet has duplicate people after merging multiple exports and you want to keep exactly one row per person while preserving the formatting of the row that remains.

## Goal
- Remove duplicate people.
- Keep one surviving row per person.
- Preserve the formatting of the surviving rows as much as possible.

## Important rule
Most spreadsheet deduplication features delete rows and keep the **first** or **last** occurrence, but they may also carry over formatting in ways that are hard to control. To avoid unexpected formatting changes, **do not use a built-in dedupe tool unless you have confirmed it will not alter the formatting you care about**.

## Safe approach
1. **Make a copy of the spreadsheet first.**
   - Work on a duplicate so you can recover if needed.

2. **Decide what identifies the same person.**
   - Prefer a stable unique field such as email address.
   - If email is missing or inconsistent, use a combination like full name + company, or another column that reliably identifies the same person.

3. **Choose the row to keep for each duplicate group.**
   - Usually keep the row with the most complete or cleanest information.
   - If one row has the preferred formatting or manual edits, keep that row.
   - If the spreadsheet has important styling differences between exports, keep the row whose formatting you want preserved.

4. **Mark duplicates before deleting anything.**
   - Add a temporary helper column such as `Keep?` or `Duplicate Group`.
   - Sort or filter by the chosen identity field.
   - For each repeated person, identify which row will be kept and which rows will be removed.

5. **Delete only the extra rows.**
   - Remove the duplicate rows one group at a time.
   - Leave the chosen surviving row untouched so its formatting stays intact.
   - If your spreadsheet app asks whether to “shift cells up” or similar, choose the option that removes the entire row.

6. **Verify the result.**
   - Confirm that each person appears only once.
   - Check several surviving rows to make sure their formatting is unchanged.
   - Spot-check merged cells, borders, fills, font styles, and formulas if those matter.

## If the spreadsheet app has a dedupe feature
Only use it if you can verify it deletes whole rows without copying formatting from another row or reformatting the sheet. If that is uncertain, use the manual approach above.

## If you need to preserve the exact formatting from one specific duplicate
- Identify that exact row first.
- Copy any non-format values you need from the other duplicates into that row.
- Delete the other duplicate rows.
- Do not paste whole rows unless you intentionally want to replace formatting.

## Recommended workflow for large sheets
- Sort by the unique identifier.
- Use filters to show duplicates.
- Keep a log of which row is retained for each person.
- Remove duplicates in batches, checking formatting after each batch.

## What this skill cannot do
This skill cannot directly operate a spreadsheet without access to the file and spreadsheet tool. It provides the safe process to follow once you have the spreadsheet open.
