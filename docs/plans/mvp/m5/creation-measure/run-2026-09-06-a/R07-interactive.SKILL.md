---
name: python-docstring-google-style-fixer
description: Fix missing Python function docstrings into Google style. Use when you have Python function code or a function excerpt that needs clearer documentation without changing the function logic.
---

# Purpose

Add or repair docstrings for Python functions so they follow Google style.
Use this skill when you are given Python function code or a function excerpt and need documentation filled in without changing the function logic.

# What to do

1. Read the function carefully.
2. Preserve the function’s code exactly unless a docstring must be inserted or replaced.
3. Write a Google-style docstring that reflects only what can be inferred from the code.
4. Include sections only when supported by the function:
   - `Args:` for parameters
   - `Returns:` for non-`None` return values or when the return value is meaningful
   - `Raises:` for exceptions that are explicit in the code and can be identified with confidence
5. If something cannot be determined from the code, do not invent it. Either omit the section or state the uncertainty in the response text outside the code, if the task requires explanation.

# Google-style docstring rules

- Start with a short one-line summary.
- Add a blank line after the summary if more detail follows.
- Document each parameter under `Args:` with its type when it can be inferred and a concise description.
- Document the return value under `Returns:` with its type when it can be inferred and a concise description.
- Document raised exceptions under `Raises:` only when the code explicitly shows them or they are strongly implied.
- Keep wording concise and factual.

# Output behavior

- Return the function with the completed docstring.
- Do not change business logic, control flow, variable names, or formatting outside the docstring unless needed to insert the docstring correctly.
- If the source already has a partial docstring, complete it in Google style rather than replacing unrelated code.
- If the source contains multiple functions, process each one independently.

# Quality checks

Before finishing, verify that:

- The docstring matches Google style.
- Every documented parameter exists in the function signature.
- No undocumented behavior is invented.
- The function body is unchanged except for docstring insertion or replacement.