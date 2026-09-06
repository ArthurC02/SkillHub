---
name: python-docstring-google-cleanup
description: Cleans up Python function docstrings into Google style when given function source code with incomplete or inconsistent documentation.
---

# Python Docstring Google Cleanup

## Purpose
Update the docstring of a Python function to Google style while preserving the function’s code and behavior.

## When to use
Use this skill when the input is Python function source code and the docstring is missing pieces, poorly formatted, or not in Google style.

## Instructions
1. Read the Python function source code exactly as given.
2. Identify the existing docstring, if any, and keep the function body and signature unchanged.
3. Rewrite only the docstring so it follows Google style.
4. Add missing `Args:` and `Returns:` sections when the function input provides enough information to do so.
5. Preserve any existing docstring content that is still accurate.
6. Do not invent behavior, parameter types, or return details that are not supported by the input.
7. If the input does not provide enough information for a docstring detail, write `not given`.
8. Return the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
9. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write `not given` where it is silent.

## Output
Return the revised Python code only, with the docstring updated to Google style.

## Procedure
1. Inspect the function signature and existing docstring.
2. Rewrite the docstring in Google style.
3. Keep the rest of the source code unchanged.
4. Output the complete revised code.

## Style requirements
- Use a one-line summary when possible.
- Add a blank line after the summary.
- Use `Args:` for parameters.
- Use `Returns:` for return values.
- Do not add unsupported sections.
- Keep wording concise and factual.

## Examples
Input:
```python
def add(a, b):
    """add two numbers"""
    return a + b
```

Output:
```python
def add(a, b):
    """Add two numbers.

    Args:
        a: not given.
        b: not given.

    Returns:
        not given.
    """
    return a + b
```