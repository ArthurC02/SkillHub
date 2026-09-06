---
name: python-docstring-google-fixer
description: Fix or complete Python function docstrings in Google style when the user pastes Python source code and wants the docstring repaired without changing function logic.
---

# Overview
You repair Python function docstrings and output the revised source code. Use this skill when the user provides Python code and asks to fill missing docstring content or convert docstrings to Google style.

## Rules
Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## What to do
1. Read the Python source the user provided.
2. Find each function docstring that needs repair.
3. Keep the function body unchanged.
4. Rewrite only the docstring so it follows Google style.
5. Add missing docstring parts when the source clearly gives enough information.
6. If the source does not provide enough information for a docstring detail, write `not given` rather than inventing it.

## Google-style docstring shape
Use the standard Google sections that fit the function:
- `Args:` for parameters.
- `Returns:` for return values.
- `Yields:` for generators.
- `Raises:` for documented exceptions when the input explicitly mentions them.
- `Examples:` only if the input already includes examples or a clear example block.

Keep the wording concise and factual. Do not add behavior, side effects, or examples that are not supported by the input.

## Output requirements
- Return only the revised source code snippet with docstrings repaired.
- Keep every function body unchanged and do not append any extra explanation, suggestions, or notes.
- Output only the revised code or docstring content requested by the user; do not add suggestions, alternatives, or commentary outside the requested repair.
- Do not change the function implementation.
- Do not introduce unrelated text before or after the code.
- If a requested docstring detail is not given in the input, use `not given`.
- If the input does not include enough material to identify the function or its docstring, respond only with the missing information you need.