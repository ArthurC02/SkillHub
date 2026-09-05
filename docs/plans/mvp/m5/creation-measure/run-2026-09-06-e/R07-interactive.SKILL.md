---
name: python-docstring-google-reviewer
description: Review a Python function’s docstring and fill in missing parts in Google style. Use when you paste a single Python function or function snippet and want a docstring draft that preserves behavior and flags anything the code does not make explicit.
---

# Python Docstring Google Reviewer

## Purpose
Review a single Python function or a small snippet centered on one function, then produce a revised docstring draft in Google style. Use this skill when the input is missing a docstring or has an incomplete one and you want the missing parts filled in without changing the function’s code.

## What to do
1. Read the function signature, body, existing docstring, and any surrounding comments that clearly apply to the function.
2. Determine the function’s purpose from its name, parameters, return statements, raised exceptions, and surrounding context if it is directly relevant.
3. Rewrite or complete the docstring in Google style.
4. Include only sections that are supported by the code:
   - `Args:` for each parameter that the function accepts.
   - `Returns:` when the function returns a value or the body makes the return behavior clear.
   - `Raises:` when the code clearly raises a specific exception type or the function contract is explicit from the code.
5. If the code does not make a detail clear, say so in the docstring draft rather than inventing a precise claim.
6. Do not modify the function code. Output only the docstring draft or the review result requested by the user.

## Writing rules
- Follow Google docstring style.
- Keep the language consistent with the source and the user’s request.
- Be concise but complete.
- Do not add examples unless the code or user request clearly supports them.
- Do not claim behavior that cannot be inferred from the input.
- If the input is not a single function or a function-centered snippet, state that the input is out of scope and explain what is missing.
- If the function has no parameters, do not add an `Args:` section.
- If return behavior cannot be determined, phrase `Returns:` conservatively or state that it cannot be confirmed from the code.
- If exception behavior cannot be determined, omit `Raises:` rather than guessing.

## Output shape
Return a revised docstring draft that can be inserted into the function, or a short limitation note when the input cannot be reviewed reliably.

## Quality check
Before finalizing, verify that:
- The docstring matches the function’s purpose.
- All parameters are represented when present.
- Return information is included only when supported.
- Exception information is included only when supported.
- No part of the function code was changed.
- Any uncertainty is explicitly marked instead of inferred.