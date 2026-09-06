---
name: python-docstring-google-style-reviewer
description: Review a Python function docstring, fill in missing parts, and rewrite it in Google style. Use when you have a Python function and want a complete, formatted docstring.
---

# Purpose
Review a Python function docstring, fill in what is missing, and rewrite it in Google style.

Use this Skill when the input is a Python function or function snippet and the goal is to produce a complete Google-style docstring.

# Instructions

1. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
2. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
3. Read the Python function and its existing docstring.
4. Keep the function’s meaning and behavior unchanged; only edit the docstring.
5. Rewrite the docstring in Google style.
6. Include the sections that are supported by the input:
   - `Args:` for documented parameters.
   - `Returns:` when the function returns a value.
   - `Raises:` when the function raises an exception that is visible in the input.
7. If a required detail is missing from the input, write `not given` rather than inventing it.
8. Preserve any factual names, parameter order, return shape, and exception types that the input provides.
9. Output only the finished docstring or the finished docstring in context if the input asks for that; do not explain the process.
10. If the input does not include enough information to produce a faithful docstring, say what is missing using `not given` and keep the output limited to the docstring.

# Output rules

- Use Google-style indentation and section labels.
- Do not add claims that are not supported by the input.
- Do not change the function body.
- Do not ask follow-up questions unless the input itself is missing the function or docstring content.
- When an item is silent, use `not given`.

# Procedure

1. Inspect the function signature, body, and current docstring.
2. Determine which parameters need `Args:` entries from the signature.
3. Determine the return description from the function body and any existing docstring.
4. Determine any visible exceptions from the function body and any existing docstring.
5. Rewrite the docstring in Google style.
6. Return the completed docstring exactly as the output artifact.