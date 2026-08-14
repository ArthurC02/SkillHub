---
name: csv-to-json
description: Convert between CSV and JSON formats — CSV to JSON array, CSV to JSONL, JSON to CSV, JSONL to CSV. Handles type inference, header/record mapping, nested structure flattening, and encoding issues. Use when the user wants to reformat tabular data between row-oriented CSV and object-oriented JSON forms.
---

# CSV ↔ JSON Conversions

Bidirectional conversion between CSV and JSON/JSONL.

## Supported directions

- **CSV → JSON array** — single file containing a JSON array of objects.
- **CSV → JSONL** — one JSON object per line (streaming-friendly).
- **JSON → CSV** — flatten JSON array of objects to CSV.
- **JSONL → CSV** — streaming line-by-line conversion.

## Procedure

1. **Confirm source and target** — file paths and directions.
2. **Detect CSV dialect** (if CSV is the source):
   - Delimiter (`,`, `;`, `\t`, `|`) — use `csv.Sniffer` or ask user if ambiguous.
   - Quote character (`"`, `'`).
   - Header present? Assume yes; confirm if the first row looks like data.
   - Encoding — try `utf-8`, fall back to `utf-8-sig` (BOM), then `cp1252` / `latin-1`. Report the encoding used.
3. **Type inference for CSV → JSON**:
   - Default: preserve as strings (safest).
   - Optional: infer types (int, float, bool, null) — ask the user. Use pandas' inference or explicit conversion.
   - Handle nulls: empty string, `NA`, `null`, `NaN` → JSON `null` (confirm the sentinel list with user).
4. **Nested structures for JSON → CSV**:
   - If JSON has nested objects/arrays, offer:
     - **Flatten** with dotted keys (e.g. `address.city`, `tags[0]`).
     - **JSON-encode** nested fields as strings in the CSV.
     - **Error** — refuse and point to `json-restructure` for explicit flattening.
   - Default: flatten with dotted keys; confirm with user.
5. **Write the output** with a sensible default name (`<stem>.json`, `<stem>.jsonl`, `<stem>.csv`).
6. **Validate the round-trip** — load the output and report row/object count matching the source.

## Dependencies

```bash
pip install pandas
```

Standard library `csv` and `json` are sufficient for many cases; pandas is convenient for type inference and large files.

## Edge cases

- **Very large files** — use streaming (JSONL line-by-line, CSV chunked via `pd.read_csv(chunksize=...)`). Warn if input > 1 GB and recommend JSONL over JSON array.
- **Mixed types per column** — pandas may coerce; report columns that ended up as `object` when numeric was expected.
- **Fields containing newlines** — CSV must quote them; JSON handles natively. Verify round-trip preserves content.
- **Date / datetime** — JSON has no native datetime type. Emit ISO 8601 strings; record the format in the data dictionary.
