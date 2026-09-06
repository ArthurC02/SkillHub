---
name: python-docstring-google-filler
description: Review a pasted Python function and fill in missing docstring content in Google style. Use this when you have Python code with an incomplete or absent docstring and want a ready-to-paste docstring or full function rewrite.
---

# Python Docstring Google Filler

You review a pasted Python function and fill in any missing docstring content in **Google style**.

## When to use this skill

Use this skill when the user provides Python function source code and asks to complete, repair, or rewrite the docstring in Google style. The function may already have a partial docstring or may have none.

## What to produce

- If the user asks for the docstring only, output a complete Google-style docstring that can be pasted into the function.
- If the user asks for the full function, output the function with the repaired or added docstring in place.
- Preserve the original function behavior and wording as much as possible.

## How to work

1. Read the function exactly as provided.
2. Identify the function name, parameters, return value, and any documented exceptions only from the code and surrounding text.
3. Write or repair the docstring in Google style using these sections when applicable:
   - short summary
   - Args:
   - Returns:
   - Raises:
   - Yields:
   - Attributes:
   - Example:
4. Only document behavior that is supported by the provided code or user text.
5. Do not guess parameter meaning, return semantics, side effects, or exceptions that are not evident.
6. Keep the output concise and directly usable.

## Style rules

- Use a one-line summary at the top when possible.
- Add an Args section for every parameter.
- Add a Returns section when the function returns a value.
- Add a Raises section only when the code or user text clearly shows an exception may be raised.
- Use plain, accurate language.
- Do not invent extra features, examples, or guarantees.

## Output rules

- Return only the docstring or the full function if requested.
- Do not explain your reasoning unless the user asks for it.
- Do not ask follow-up questions unless the input is missing the function source code.

## Minimal completion standard

A correct result must:

- use Google-style docstring formatting,
- include parameter documentation when parameters exist,
- include return documentation when the function returns a value,
- avoid unsupported claims,
- preserve the original function’s meaning.