---
name: python-docstring-google-fixer
description: Fill in missing Python function docstrings and normalize them to Google style when you need to patch code that lacks docs or has incomplete docstrings. Use this when given Python functions and you want the output as editable source code with docstrings added or completed.
---

# Python Docstring Google Fixer

## Purpose
Update Python function docstrings so missing documentation is filled in and existing documentation is brought to Google style, while preserving the code’s behavior and structure.

## What to do
1. Read the Python source exactly as given.
2. Find every function definition in the input.
3. For each function:
   - If it has no docstring, add a Google-style docstring immediately under the function definition.
   - If it already has a docstring, keep its meaning and update only the missing Google-style sections or field content.
   - Include only details that are directly supported by the code or the user-provided source.
4. Preserve all code outside docstrings:
   - Do not change function names.
   - Do not change parameters.
   - Do not change return logic.
   - Do not rewrite implementation code.
5. Use Google docstring structure as appropriate for the code:
   - Short summary line.
   - `Args:` for parameters.
   - `Returns:` when the function returns a value.
   - `Raises:` when the function explicitly raises exceptions.
6. If the code does not provide enough information to safely infer a detail, state only what is visible in the code rather than guessing.
7. Output the full revised Python source code and nothing else.

## Style rules
- Keep the docstring concise and directly tied to the function’s behavior.
- Use plain language.
- Match Google style formatting.
- Do not add commentary outside the code block in the final output.

## Validation target
The result should be directly pasteable back into Python source code, with only docstring changes made unless the user’s input itself requires a different adjustment.