# Draft PR: a single Han character is a searchable query

## Business intent

A Catalog searcher who types one Chinese character such as 表 or 水 gets a search, because one Han character is a complete word; one Latin letter, digit, kana or symbol is still too little to search.

## Domain impact

catalog owns the rule a-searchable-query-has-two-runes-and-a-letter-or-digit. Its statement gains the single-Han exception; no other Context, collaboration or contract changes.

## Implementation handoff

isComprehensible accepts one rune when unicode.Is(unicode.Han, r), so the extension blocks count as well as U+4E00–9FFF; rejected: a hard-coded basic-block range, and accepting any single letter. Proof: dropping the Han clause makes TestIsComprehensible fail on each of the three one-character Han queries (U+4F60, U+8868, U+20B9F), then restored.

## Proposal and approvals

single-han-query-2026-11-01, one developer approval; ruled by the Optimization session on the user's instruction to settle design questions by best practice.

## Contract impact

None. The workspace route's 400 message names the exception.

## Verification

rule-a-searchable-query-has-two-runes-and-a-letter-or-digit → TestIsComprehensible; evidence-current → verify-evidence on both cited files.

## Residual risks

The rule id still says two runes; it is kept so existing links do not break, and the statement is the source of truth.
