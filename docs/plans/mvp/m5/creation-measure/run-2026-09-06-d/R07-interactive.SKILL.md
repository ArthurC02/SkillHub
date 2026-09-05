---
name: python-docstring-google-style-auditor
description: Review Python function docstrings, fill in missing content, and normalize them to Google style. Use when you have Python code or docstring fragments that need consistent, accurate docstring cleanup without changing function behavior.
---

# Python Docstring Google Style Auditor

Review Python function docstrings, fill missing pieces, and rewrite them in Google style without changing the function’s behavior.

## When to use

Use this skill when you have:
- A Python function with an incomplete, outdated, or inconsistent docstring
- Multiple functions that need to follow the same Google-style docstring pattern
- A need to preserve code behavior while improving documentation quality

## What this skill does

1. Inspects the function signature and existing docstring.
2. Identifies documented and undocumented parameters, return values, and raised exceptions.
3. Rewrites the docstring in Google style.
4. Preserves accurate existing descriptions when they are correct and complete.
5. Flags information that cannot be inferred reliably instead of inventing details.

## Rules

- Do not change the function logic.
- Do not invent behavior, parameter meanings, return values, or exceptions that are not supported by the input.
- Only include sections that are relevant to the function.
- If a section is not needed, omit it rather than creating filler text.
- Keep the output ready to paste back into the codebase.

## Process

### 1. Read the input carefully

Inspect:
- Function name
- Parameters and default values
- Type hints, if present
- Existing docstring text
- Control flow that clearly affects return values or exceptions

### 2. Determine which Google-style sections are needed

Include only the sections justified by the function:
- `Args:` for parameters that need explanation
- `Returns:` when the function returns a value other than `None`
- `Yields:` when the function is a generator
- `Raises:` when the function explicitly raises exceptions or when the code clearly does so
- `Examples:` only when an example is supplied or can be written safely from the input

### 3. Fill missing content conservatively

- If a parameter is obvious from its name and type, describe it concisely.
- If a parameter’s role is unclear, state that the input does not provide enough information and ask for clarification outside the docstring rewrite.
- If the return value is not clear, do not guess. Leave the output focused on the parts that can be verified.
- If exceptions are not explicit in the input, do not add a `Raises:` section.

### 4. Write the final docstring in Google style

Use these formatting conventions:
- One-line summary first.
- A blank line after the summary when additional sections are present.
- Section headers followed by a colon, such as `Args:` and `Returns:`.
- Indent descriptions consistently under each section.
- Keep lines concise and readable.

### 5. Validate the result

Before returning, check that:
- The docstring matches the function signature.
- No nonexistent parameters or exceptions were introduced.
- The style is Google-style and consistently formatted.
- The wording is clear, concise, and faithful to the input.

## Output expectations

Return a docstring that can be pasted directly into the function body.
If the input does not contain enough information for a trustworthy rewrite, say exactly what is missing instead of filling gaps with guesses.