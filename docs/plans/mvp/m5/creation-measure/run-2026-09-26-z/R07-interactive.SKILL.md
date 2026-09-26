---
name: python-docstring-google-fixer
description: Review a Python function docstring, fill in missing content, and rewrite it in Google style when you are given Python code with an incomplete or non-Google docstring.
---

# Python docstring Google-style fixer

You receive Python source text that includes one or more function docstrings. Your job is to return the finished artifact itself: the revised docstring or source text with the docstring corrected, in Google style.

## What to do

1. Read the input exactly as given.
2. Identify the Python function docstring that needs review.
3. Fill in missing docstring content using only what the input contains.
4. Rewrite the docstring in Google style.
5. Preserve the function name, function signature, code body, and all non-docstring source text unless the input itself requires a change.
6. Return the finished artifact itself, ready to paste back into code.

## Required rules

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Google style requirements

- Use a concise summary line.
- Add a blank line after the summary if additional sections are needed.
- Use Google-style section headers such as `Args:`, `Returns:`, `Yields:`, `Raises:`, and `Attributes:` only when the input supports them.
- Describe each parameter from the function signature when the input provides enough information to do so.
- If the input does not provide enough information for a field or section, write `not given` rather than inventing details.
- Do not switch to NumPy style or reST style.

## Output behavior

- If the input is only a docstring, return the corrected docstring text.
- If the input is a full function definition, return the full function definition with the docstring corrected in place.
- Keep the output directly usable as source text.
- Do not add commentary outside the artifact.

## Working method

1. Extract the function signature and existing docstring text.
2. Determine which Google-style sections are supported by the input.
3. Rewrite the docstring so the content is complete and consistent.
4. Preserve any code outside the docstring.
5. Output only the final artifact.

## Edge cases

- If a parameter is present but its purpose is not stated in the input, describe it as `not given`.
- If a return value is present but its meaning is not stated in the input, describe it as `not given`.
- If the input does not contain enough information to rewrite safely, keep the provided text and mark the missing parts as `not given`.

## Final check

Before answering, ensure the result is a clean, paste-ready artifact and that every addition came from the input or is marked `not given`.