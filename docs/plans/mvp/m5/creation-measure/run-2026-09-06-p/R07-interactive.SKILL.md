---
name: python-docstring-google-fixer
description: Use when you are given a Python function and an incomplete or inconsistent docstring; returns a completed docstring in Google style that can be pasted back into the code.
---

# Python Docstring Google Fixer

You receive a Python function and its current docstring. Your job is to return a completed docstring in Google style that fits the given code.

## Instructions

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

1. Read the function signature and the existing docstring.
2. Preserve the function’s meaning, name, parameter names, and any facts already stated in the input.
3. Fill in missing docstring content in Google style.
4. Include only sections that are supported by the input:
   - `Args:` for parameters
   - `Returns:` when the function’s return value is evident from the code or input
   - `Raises:` only when exceptions are evident from the code or input
   - `Yields:` only when the function is clearly a generator from the code or input
5. If a section is silent in the input, write `not given` rather than inventing details.
6. Keep the wording concise, natural, and consistent with Google-style docstrings.
7. Output only the finished docstring text, ready to paste back into Python code.

## Output requirements

- Do not explain your reasoning.
- Do not discuss formatting rules.
- Do not ask follow-up questions unless the input itself is missing the function or docstring.
- Do not rewrite unrelated code.
- Do not change the function’s semantics.
- If the input does not provide enough information for a section, use `not given` in that section instead of guessing.

## Style

- Use plain, direct language.
- Prefer short descriptions.
- Keep indentation and Google docstring structure clean.
- Match the input’s level of detail without inventing new behavior.