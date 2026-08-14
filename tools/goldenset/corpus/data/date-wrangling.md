---
name: date-wrangling
description: Perform date/time format transformations on a dataset — converting between ISO 8601, epoch (seconds/millis), with-timezone, without-timezone, date-only, datetime, Unix timestamp, locale-specific display formats, and fiscal / Julian / week-number representations. Use when a dataset has dates in the wrong format for downstream use (API, SQL, ML pipeline) and needs enriching or refactoring.
---

# Date Wrangling

Transform date/time columns into the format required by the downstream consumer.

## When to invoke

- Downstream system (API, SQL column, ML feature, HF dataset) needs a specific date format and the source uses a different one.
- User asks to "convert dates to epoch", "add timezone", "strip timezone", "normalise to ISO 8601", "add a timestamp column".

## Supported transformations

| Source | Target | Notes |
|---|---|---|
| Any parseable date/datetime | **ISO 8601** (`YYYY-MM-DDTHH:MM:SS±HH:MM`) | Default canonical form |
| Any | **Epoch seconds** (Unix timestamp) | Integer seconds since 1970-01-01 UTC |
| Any | **Epoch milliseconds** | Integer ms since epoch (JS / Java convention) |
| Any | **Epoch microseconds / nanoseconds** | For high-precision logging |
| Naive datetime | **Timezone-aware** | Require user to specify the assumed source TZ |
| Timezone-aware | **UTC** | Convert to UTC and keep offset, or strip offset |
| Timezone-aware | **Naive** (strip TZ) | Warn — lossy; confirm with user |
| Date + time split | **Single datetime** | Combine two columns into one |
| Single datetime | **Date + time split** | Produce two columns |
| Any | **Locale display format** (e.g. `01/02/2024` UK) | For human-facing outputs only; never store |
| Any | **Fiscal year / quarter** | Fiscal calendar start month user-configurable |
| Any | **ISO week date** (`YYYY-Www-D`) | Useful for weekly reporting |
| Any | **Julian day / day-of-year** | Scientific applications |

## Procedure

1. **Locate the source column(s)** — confirm which columns hold dates/times. If multiple candidates, list them with sample values and ask user.
2. **Parse the source format**:
   - If uniform and parseable, use `pandas.to_datetime` with `format=...` (explicit is safer than inference).
   - If mixed, detect the formats present; do not silently dispatch — list them and confirm per-format handling.
   - Ambiguous ordering (`01/02/2024` — is it UK `DD/MM/YYYY` or US `MM/DD/YYYY`?) → **never guess**. Ask the user or consult the data dictionary.
3. **Determine the source timezone**:
   - If offset/zone is embedded, use it.
   - If naive, ask the user what TZ to assume. Common answers: UTC, local, a specific IANA zone (`Europe/London`, `Asia/Jerusalem`).
4. **Apply the target transformation**.
5. **Write output** — new column with descriptive name (`<col>_iso8601`, `<col>_epoch_ms`, `<col>_utc`), preserving the original by default. Offer overwrite only on explicit request; backup first per `CONVENTIONS.md`.
6. **Enrich if requested** — add derived columns (`year`, `month`, `day`, `week`, `day_of_week`, `is_weekend`, `is_holiday`) via the `data-enrichment` skill's temporal options.
7. **Validate** — round-trip a sample through the transformation and back to confirm no precision loss (esp. for epoch conversions).
8. **Update the data dictionary** — record the source format, target format, assumed timezone, and any lossy conversion.

## Dependencies

```bash
pip install pandas python-dateutil
# optional
pip install pytz          # legacy TZ database
# Python 3.9+ includes zoneinfo natively
```

## Edge cases

- **DST transitions** — `02:30` on a spring-forward day doesn't exist; `02:30` on a fall-back day is ambiguous. Default to raising an error with the row indices; offer user policies (skip, pick first occurrence, pick second, shift).
- **Pre-1970 dates** — epoch will be negative; confirm the target system accepts it.
- **Unix epoch vs Excel epoch** — Excel's 1900-based epoch is a common trap. Detect likely-Excel numeric dates (small values like `45678`) and ask before treating them as Unix epoch.
- **Very precise timestamps** — JavaScript's `Date` loses microseconds. Warn if source has sub-ms precision and target is JS-consumed.
- **Nanosecond overflow** — pandas `datetime64[ns]` overflows at `2262-04-11`. For dates beyond that range, use `datetime64[us]` or store as strings.
- **Fiscal calendar** — always confirm the start month with the user; don't assume January.

## Safety

This skill prefers to add new columns rather than mutate originals. When asked to overwrite, follow the backup policy in `CONVENTIONS.md` — confirm an existing backup or create one before writing.
