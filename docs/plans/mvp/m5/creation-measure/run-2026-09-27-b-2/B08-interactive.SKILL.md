---
name: invoice-reconciliation-organizer
description: Organize invoice details into a structured reconciliation output for accounting-system upload or human review. Use it when the user provides invoice data that needs deduping, missing-field marking, and a ready-to-check summary.
---

# Invoice Reconciliation Organizer

## Purpose
Turn raw invoice details into a structured reconciliation result that can be uploaded later or reviewed by a person. Use this skill when the user gives invoice text, a table, or mixed invoice notes and wants them organized for monthly reimbursement or accounting review.

## What to produce
Output a clean, structured reconciliation artifact in Traditional Chinese unless the user explicitly asks for another language. Prefer a table or bullet list that keeps each invoice separate and easy to compare.

For each invoice, include the following when given:
- date
- vendor or merchant name
- invoice number, if given
- amount
- tax amount or tax-inclusive status
- item, purpose, or expense category
- duplicate or suspicious duplicate note, if relevant
- missing-field note, if relevant

If the user asks for a specific output format, follow that format. If no format is given, use a markdown table plus a short review summary.

## Core procedure
1. Read the invoice material exactly as provided.
2. Separate the material into individual invoice records.
3. Extract only facts that are present in the input.
4. Mark missing fields clearly.
5. Flag possible duplicates when the same invoice appears more than once or when the details are materially identical.
6. Produce the finished reconciliation artifact directly in the output.
7. If the user asked for upload or submission, prepare the content ready for use and say plainly that sending or scheduling is left to the person.

## Required rules
when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently; never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output guidance
- Keep all extracted facts tied to the source material.
- Do not infer tax calculations unless the user explicitly asks for calculations and the needed inputs are present.
- If the input contains repeated records, keep both records in the result and label the duplication.
- If fields are missing, mark them as "not given" only for missing facts.
- If the user wants the result formatted for a specific accounting system, preserve the system's requested column order when supplied.

## Default format when none is provided
Use this structure:

| Record | Date | Vendor | Invoice No. | Amount | Tax / Tax-inclusive | Item / Purpose | Notes |
|---|---|---:|---|---:|---|---|---|
| 1 | ... | ... | ... | ... | ... | ... | ... |

Then add:
- Duplicate check: ...
- Missing fields: ...
- Ready for upload: ...

## Quality check before finishing
- Every invoice in the input appears once in the output, unless the user explicitly requested filtering.
- No unsupported facts were added.
- Missing information is marked, not guessed.
- Any duplicate is called out.
- The output is ready to use without further explanation.
