---
name: spreadsheet-keep-headers-and-formatting-consistent
description: Use when a spreadsheet has many rows and you need the header to remain visible while scrolling, and the columns/number formatting to stay readable and consistent. It helps you freeze the top row and standardize widths and alignment, but it does not perform live edits without spreadsheet access.
---

# Keep a large stock sheet readable

Use this skill when a spreadsheet is hard to read because the header scrolls away and the columns have inconsistent widths or alignment.

## What to do

1. **Freeze the header row**
   - Freeze the first row so it stays visible while you scroll.
   - If there is more than one header row, freeze all header rows, not just the top one.
   - If the sheet has a title row above the headers, decide whether that title should also remain frozen or whether the actual column headers are the only rows that need to stay visible.

2. **Standardize column widths**
   - Make widths consistent where the data type is the same across adjacent columns.
   - Give text-heavy columns more space than code/ID columns.
   - Avoid overly narrow columns that force truncation or wrapping.
   - Avoid excessive width that creates empty whitespace.
   - If the sheet is long and mostly tabular, prefer a clean, uniform width pattern over ad hoc sizing.

3. **Align by data type**
   - Left-align text fields such as names, descriptions, categories, and notes.
   - Right-align numeric fields such as quantities, prices, counts, percentages, and dates if the spreadsheet convention in use supports it.
   - Keep headers aligned consistently with the column they label unless the sheet’s style guide says otherwise.
   - If some numbers are left-aligned and others are right-aligned, normalize them so the same kind of data uses the same alignment everywhere.

4. **Apply consistent number formatting**
   - Use one format for each type of numeric data:
     - integers for counts,
     - fixed decimals for measurements or prices where precision matters,
     - currency format for money,
     - percent format for rates.
   - Make sure all cells in a column follow the same format.
   - If some values are stored as text, convert them to proper numbers when possible so sorting and alignment work correctly.

5. **Use a consistent table style**
   - Keep font, size, and header styling uniform across the sheet.
   - Use clear header emphasis, such as bold text or a fill color, so the frozen row is easy to identify.
   - If the sheet supports banded rows, use subtle banding to improve scanability.
   - Avoid mixing multiple visual styles unless they encode different meanings.

6. **Check the result for readability**
   - Scroll through several sections of the sheet and confirm the header remains visible.
   - Verify that every column’s width and alignment make the data easy to scan.
   - Confirm that rows still fit on screen reasonably well and that no important content is hidden.

## If you are only advising, not editing

Explain that the ideal setup is:
- freeze the top header row,
- set column widths intentionally,
- align text and numbers consistently,
- apply one number format per data type.

If the user asks for exact steps, ask which spreadsheet app they use, because the clicks differ between Excel, Google Sheets, and similar tools.
