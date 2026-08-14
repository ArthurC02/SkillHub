---
name: pii-flag
description: Scan a dataset for personally identifiable information (PII) — names, emails, phone numbers, addresses, government IDs, credit cards, IPs, dates of birth, geocoordinates — and produce a cell-level report of where PII was detected, with confidence scores and recommended remediation. Use before publishing, sharing, or pushing a dataset to public storage (e.g. Hugging Face).
---

# PII Flag

Detect PII in a dataset at column and cell level, and report findings.

## When to invoke

- Before `hf-dataset-push` or any public distribution.
- User asks "does this have PII?", "is this safe to share?", "scan for personal data".

## Detectable PII categories

- **Names** — person names (first / last / full).
- **Emails** — regex + validation.
- **Phone numbers** — international formats, via `phonenumbers`.
- **Postal addresses** — street, postcode, city combos.
- **Government IDs** — SSN (US), NI (UK), teudat zehut (IL), TIN, passport — per-country patterns.
- **Credit card numbers** — Luhn-validated.
- **IP addresses** — IPv4 / IPv6.
- **Dates of birth** — date fields with a DOB-like column name or value distribution.
- **Geocoordinates** — lat/long pairs at high precision.
- **Medical IDs / health data** — if detected, flag as special-category (GDPR Article 9).
- **Free text containing the above** — run detection on text columns, not just structured ones.

## Procedure

1. **Profile columns** — names, dtypes, samples.
2. **Rule-based pass per column**:
   - Header-name heuristics: columns called `email`, `phone`, `ssn`, `address`, `dob`, `name`, `first_name`, `last_name`, `ip`, `lat`, `lon` get flagged up-front.
   - Value-pattern heuristics: regexes for emails, phones, CC numbers, IPs; `phonenumbers` for validation; `python-stdnum` for national IDs.
3. **ML-based pass on text columns** (optional, ask user) — use `presidio-analyzer` or a local NER model to catch PII inside free text.
4. **Per-cell report** — produce a file `pii_report.jsonl` with one line per detected cell:
   ```json
   {"row": 42, "column": "notes", "start": 18, "end": 34, "category": "EMAIL", "value": "a***@example.com", "confidence": 0.98}
   ```
   Mask the reported value by default (show first char + asterisks) — do **not** echo full PII into reports.
5. **Column-level summary** — for each column, list categories detected, count, and worst-case sample count.
6. **Remediation recommendations** per column:
   - **Drop** (column not needed for downstream use).
   - **Hash / tokenise** (pseudonymise with a salted hash).
   - **Generalise** (bucket DOB to age range; truncate postcode; round coordinates to 0.1°).
   - **Replace with synthetic** (see `synthetic-data-overlay`).
   - **Keep** (explicit user decision, with rationale recorded).
7. **Stop if catastrophic** — if the dataset contains special-category or high-risk PII (health, government IDs, credit cards), warn the user loudly and **require explicit confirmation** before any downstream publication steps.

## Dependencies

```bash
pip install pandas phonenumbers python-stdnum
# optional ML-based detection
pip install presidio-analyzer presidio-anonymizer
```

## Edge cases

- **False positives** — short numeric strings, test data, dummy emails (`example.com`, `test@test.com`). Report but flag as likely-benign; don't auto-remediate.
- **PII in column names** themselves — rare but possible; flag.
- **Encoded PII** — base64 / URL-encoded / hashed. Do not attempt to reverse; flag as "possibly encoded".
- **Non-English text** — ensure detection runs on Unicode; some regexes are ASCII-only. Use `phonenumbers` with locale hints.
- **The report itself is sensitive** — remind the user not to share `pii_report.jsonl` publicly; default to writing it alongside the dataset with a `.gitignore` entry.
