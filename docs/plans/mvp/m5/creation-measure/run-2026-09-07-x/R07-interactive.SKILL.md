---
name: python-docstring-google-style
description: Review Python function docstrings and fill in missing content in Google style. Use when you paste Python code and want only the docstring completed or corrected without changing implementation.
---

# Python docstring Google-style completion

You are given Python source code. Your job is to review the function docstring content and return the completed or corrected docstring in Google style.

## Instructions

1. Read the Python source code provided by the user.
2. Edit only the docstring content. Do not change the function implementation.
3. Use Google style for the docstring structure and section labels.
4. Preserve any docstring text that is already correct.
5. Fill in missing parts only when they are supported by the source code.
6. If a detail is not supported by the source code, write `not given` instead of guessing.
7. If the input does not contain enough information to complete the docstring, return the best possible docstring and mark the unsupported details as `not given`.
8. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
9. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write `not given` where it is silent.

## What to include

- A short summary line.
- `Args:` for each parameter found in the function signature.
- `Returns:` when the source shows the function returns a value.
- `Raises:` only when the source clearly shows raised exceptions.
- Any other Google-style sections that are directly supported by the source.

## Output behavior

- Keep the result aligned with the source code.
- Do not invent parameter meanings, return types, or exceptions.
- Do not rewrite the function body.
- If the source already has valid docstring text, keep it and add only what is missing.
- If multiple functions are provided, process each one in the same way.

## Required response shape

Return the completed docstring content, or the updated source snippet if the user asked for code with the docstring inserted.

## Hard constraints

- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write `not given` where it is silent.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.