---
name: python-docstring-google-reviewer
description: Review Python function docstrings, fill in missing details, and rewrite them in Google style. Use when you paste Python functions and want a concise docstring audit with safe completion suggestions.
---

# Python Docstring Google Reviewer

Review Python function docstrings, fill in missing information when it can be inferred safely, and rewrite the result in Google style.

## When to use

Use this skill when you have one or more Python functions and want:

- a docstring review,
- missing docstring sections filled in,
- a Google-style docstring rewrite,
- a clear note when the function does not provide enough information to infer something safely.

## Instructions

1. Read the function signature, existing docstring, and any nearby code that is directly relevant to the function’s behavior.
2. Identify the function’s purpose, parameters, return value, raised exceptions, side effects, and important edge cases.
3. Compare the existing docstring against Google style and note what is missing or inconsistent.
4. Rewrite the docstring in Google style with these rules:
   - Start with a short summary line.
   - Add `Args:` for parameters when the function accepts parameters.
   - Add `Returns:` when the function returns a value.
   - Add `Raises:` when the function can raise relevant exceptions.
   - Include `Yields:` for generator functions instead of `Returns:`.
   - Include `Attributes:` only for class docstrings, not plain functions.
5. If a detail cannot be inferred safely from the signature and surrounding code, do not invent it. State that the information is missing or ambiguous.
6. Keep the scope limited to docstring review and replacement suggestions. Do not refactor the function body or rewrite unrelated code.

## Output format

For each function, provide:

1. A brief review of what is missing or inconsistent in the current docstring.
2. A Google-style docstring rewrite.
3. A short note for any uncertain or missing information that prevented a complete rewrite.

## Quality rules

- Preserve the function’s actual behavior.
- Prefer literal, code-supported wording over guesses.
- Do not add parameters, returns, or exceptions that are not supported by the code.
- If the function has no docstring, create one from the available code evidence and mark any uncertain parts clearly.
- Keep the response focused on documentation only.