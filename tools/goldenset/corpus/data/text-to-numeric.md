---
name: text-to-numeric
description: Convert text-formatted numeric values (e.g. "$4.27", "1,234.56", "€1.2M", "3%", "(500)") into clean numeric columns, recording the original formatting (currency, scale, sign convention) in the data dictionary. Use when a column that should be numeric is typed as string because of embedded symbols, separators, or scale suffixes.
---

# Text to Numeric

Parse formatted numeric strings into proper numeric types, capturing the format metadata separately.

## When to invoke

- A column contains values like `$4.27`, `1,234.56`, `€1.2M`, `3.5%`, `(500)` (accounting negative), `2.5K`.
- Downstream analysis needs the column as `float` / `int`.

## Procedure

1. **Load dataset** and identify the target column(s). If the user hasn't specified, list columns where `dtype == object` but >80% of values look numeric after stripping symbols.
2. **Profile the column** — sample 20 distinct values and detect format:
   - Currency symbols: `$`, `€`, `£`, `¥`, `₪`, `₹`, etc. → record the detected currency.
   - Thousands separators: `,` (US), `.` (EU), space (FR/scientific), `'` (CH).
   - Decimal marker: `.` or `,` — infer from context; ask if ambiguous.
   - Scale suffixes: `K`, `M`, `B`, `T` (multiply accordingly).
   - Percentage: `%` → divide by 100 OR keep as-is; ask the user.
   - Accounting negatives: `(500)` → `-500`.
   - Unicode minus: `−` → `-`.
3. **Confirm the detected format with the user** before applying, especially the decimal/thousands convention and percentage handling.
4. **Parse**:
   - Strip currency symbols, whitespace, and thousands separators.
   - Apply scale suffix multipliers.
   - Convert accounting negatives.
   - Cast to `float`; downcast to `int` if all values are whole numbers.
5. **Write two columns** (default):
   - Original column preserved (renamed to `<col>_raw` if the user wants a clean replacement) OR overwritten.
   - New numeric column `<col>_numeric` (or same name).
6. **Record metadata in the data dictionary**:
   - Original format (e.g. "US currency, $ prefix, comma thousands, dot decimal").
   - Detected currency ISO 4217 code if present.
   - Scale convention applied.
   - Percentage handling decision.
   - If no data dictionary exists in the dataset's folder, create one via the `add-data-dictionary` skill.
7. **Report** unparseable rows — list values that failed parsing with their row indices. Leave null in numeric column; do not drop.
8. **Write output** with `_numeric` suffix.

## Dependencies

```bash
pip install pandas
```

Optional: `babel.numbers.parse_decimal` for locale-aware parsing.

## Edge cases

- **Mixed currencies in one column** (e.g. `$5`, `€4`, `£3`) — extract currency per row into a separate `currency` column, convert numeric without the symbol. Do **not** attempt FX conversion; that is a separate operation.
- **Ranges** (`"100-200"`, `"5–10"`) — flag and ask user (split into `_min`/`_max`, take midpoint, or leave as text).
- **Approximations** (`"~500"`, `"<100"`, `">1M"`) — strip qualifier, record in a `qualifier` column, convert the numeric portion.
- **Scientific notation** — pandas handles natively.
- **NaN sentinels** (`"N/A"`, `"-"`, `"null"`, `""`) — convert to NaN; list the sentinels detected.
