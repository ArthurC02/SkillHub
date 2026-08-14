---
name: data-cleanliness-scan
description: Scan one or more flat data files (CSV, Parquet, JSON, JSONL, Excel) to assess data cleanliness and identify columns likely to fail SQL ingestion — inconsistent types, mixed delimiters, malformed dates, nullability mismatches, duplicate keys, encoding issues, and out-of-range values. Produces a ranked issue report with concrete remediation suggestions.
---

# Data Cleanliness Scan

Audit flat data files and flag issues that would block SQL ingestion or cause analytical errors.

## When to invoke

- Before `sql-load` to catch issues early.
- User asks "is this data clean?", "what's wrong with this dataset?", "will this load into Postgres?".
- After receiving a new data dump from an external source.

## Checks performed

### Per column

1. **Type consistency** — in an `object`/string column that *should* be numeric or date, flag rows that don't conform and report the fraction.
2. **Mixed types** — same column has ints, floats, strings, bools interleaved.
3. **Null sentinels** — detect disguised nulls (`"N/A"`, `"null"`, `"-"`, `""`, `"#N/A"`) that are not real NaN.
4. **Date parsing** — detect date-shaped strings; check for mixed formats (`DD/MM/YYYY` vs `MM/DD/YYYY` vs `YYYY-MM-DD`). Flag ambiguous orderings.
5. **Whitespace** — leading/trailing whitespace, tab characters, non-breaking spaces in cells.
6. **Case inconsistency** — for low-cardinality columns, report `{"Active": 42, "active": 17, "ACTIVE": 3}`.
7. **Encoding artefacts** — mojibake patterns (`Ã©` for `é`, `â€™` for `'`).
8. **Out-of-range values** — negative ages, future dates in historical columns, percentages > 100.
9. **Outliers** — > 5σ from mean for numeric columns (as a flag, not a removal recommendation).
10. **Nullability** — columns that should be non-null (e.g. keys) with nulls present.
11. **Uniqueness** — candidate key columns with duplicates.
12. **Length / format** — codes with inconsistent length (e.g. `country_code` mixing `US` and `USA`).

### Per file

1. **Row count mismatch** between header and data (for CSV).
2. **Delimiter drift** — file declared CSV but some rows use semicolons.
3. **Ragged rows** — different column counts per row.
4. **BOM presence** — UTF-8 with BOM vs without.
5. **Line-ending mix** — CRLF and LF interleaved.

### Cross-column

1. **Referential integrity** hints — `country_code` populated but `country_name` null, or vice versa.
2. **Logical consistency** — `end_date < start_date`, `total != sum(components)` when components are present.

## Procedure

1. **Determine input(s)** — one or several flat files. Confirm formats and target.
2. **Run the checks above** — use pandas for profiling; add `chardet` for encoding, `dateutil` for date parsing attempts.
3. **Rank findings by severity**:
   - **Blocker** — will definitely fail SQL ingestion (mixed types, ragged rows, duplicate PKs, invalid dates).
   - **High** — likely to cause analytical errors (mixed date formats, disguised nulls, out-of-range values).
   - **Medium** — inconsistencies that muddy results (case mismatch, whitespace, mojibake).
   - **Low** — cosmetic (trailing whitespace in some cells).
4. **Produce the report** `cleanliness_report.md`:
   - Executive summary — file names, row counts, blocker count, high count.
   - Per-file section listing findings with severity, row count affected, sample cells (masked for PII), and recommended remediation skill (`standardise-country-names`, `text-to-numeric`, etc.).
   - Optional JSON twin `cleanliness_report.json` for programmatic consumption.
5. **Offer to invoke remediation skills** in the order suggested — but do not run them automatically.

## Dependencies

```bash
pip install pandas pyarrow openpyxl chardet python-dateutil
```

## Edge cases

- **Very large files** — sample strategically (first/last/random N rows + full-file aggregations via streaming). Note the sample size in the report.
- **Heavily nested JSON** — structural cleanliness (schema consistency across records) is the main check; recommend `json-restructure` to flatten before deeper analysis.
- **Genuinely heterogeneous columns** (e.g. `value` column in an EAV table) — recognise the pattern and treat it differently rather than flagging as "mixed types".
- **False positives on outliers** — outliers are not always errors. Report as observations, not defects; let the user decide.
