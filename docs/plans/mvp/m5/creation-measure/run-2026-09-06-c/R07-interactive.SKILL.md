---
name: python-docstring-google-fixer
description: Review Python function docstrings, fill in missing details, and normalize them to Google style. Use when you paste Python functions and want a docstring-only rewrite without changing code behavior.
---

# Python Docstring Google Fixer

## Purpose
Use this skill when you have Python function code and want its docstring reviewed, completed, or reformatted into Google style.

## What to do
1. Read the function signature, body, and any existing docstring.
2. Identify which docstring parts are supported by the code:
   - short summary
   - extended summary, if useful
   - `Args`
   - `Returns`
   - `Yields`
   - `Raises`
   - `Attributes`, only when documenting a class, not a function
   - `Examples`, only if the code or request provides enough evidence
3. Rewrite the docstring in Google style.
4. Keep the meaning aligned with the code.
5. Do not invent parameters, return values, exceptions, side effects, or usage details that are not supported by the code or the user-provided input.
6. Do not change the function implementation.

## Editing rules
- Preserve correct technical meaning.
- Prefer concise, factual wording.
- If the original docstring is already close to Google style, keep its intent and only fix structure, missing sections, and unclear wording.
- If a function has no docstring, generate a complete Google-style docstring from the code evidence.
- If a detail cannot be inferred safely, omit it rather than guessing.
- Match the actual control flow for `Returns`, `Yields`, and `Raises`.

## Google-style structure
Use the sections that apply to the function:

```python
"""Short summary.

Optional longer explanation.

Args:
    name: Description.
    other_name: Description.

Returns:
    Description of the return value.

Raises:
    ValueError: When ...
"""
```

## Checklist before finalizing
- The summary matches the function purpose.
- Every documented parameter exists in the signature.
- Every documented return value matches the actual behavior.
- Every documented exception is supported by the code or the user input.
- Formatting follows Google style.
- No implementation code was changed.

## Output expectations
Return the revised docstring or the rewritten function block as requested by the user. If the user asks for only the docstring, output only the docstring text in Google style.