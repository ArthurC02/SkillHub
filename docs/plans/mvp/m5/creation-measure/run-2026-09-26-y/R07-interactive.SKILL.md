---
name: python-docstring-google-completer
description: Complete or repair Python function docstrings into Google style when given a function body and any existing docstring. Use when you need the docstring rewritten or filled in from the code itself.
---

# Python Docstring Google Completer

You rewrite a Python function’s docstring into Google style, using only the information present in the input.

Use this skill when the user provides Python function code and asks to complete, repair, or reformat its docstring into Google style.

## Instructions

1. Read the Python function and any existing docstring.
2. Identify what the function text itself states about purpose, parameters, return value, and raised exceptions.
3. Write a Google-style docstring that preserves the function’s meaning and stays consistent with the code.
4. Include only sections supported by the input.
5. If a detail is not stated in the input, write `not given`.
6. Output the finished docstring only.

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output requirements

- Return a single Google-style docstring.
- Keep the function’s meaning unchanged.
- Do not add explanations, commentary, or analysis.
- Do not invent parameters, return values, or exceptions that are not supported by the input.

## Google style shape

Use the standard Google docstring sections as supported by the input:

- `Args:` for parameters
- `Returns:` for return values
- `Raises:` for exceptions

If the input does not support a section, omit that section.

## Handling missing information

- If the input does not state a value that is needed for the docstring, write `not given`.
- If the input does not support a section, do not invent one.
- If the existing docstring is partial, complete it without changing the function’s code.

## What to preserve

- Preserve the function’s intent.
- Preserve any truthful details already present in the input.
- Preserve the code’s observable behavior as described by the input.

## What not to do

- Do not change the Python code.
- Do not add extra prose before or after the docstring.
- Do not infer behavior that the input does not show.
- Do not ask the user follow-up questions unless the input itself is missing and the missing detail prevents completion.