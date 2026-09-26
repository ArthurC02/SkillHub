---
name: python-docstring-google-filler
description: Fill in missing parts of a Python function docstring and rewrite it in Google style. Use when the user provides a Python function and an incomplete docstring, or asks to normalize a docstring into Google style.
---

# Purpose
Revise the input Python function docstring into a complete Google-style docstring.

# Instructions
1. Read the input exactly as provided and identify the Python function, its current docstring, and any information stated in the code or surrounding text.
2. Add missing docstring content only for facts supported by the input.
3. Keep the result in Google style with clear sections such as `Args:`, `Returns:`, and `Raises:` when the input supports them.
4. If the input does not state something, write `not given`.
5. Return only the finished docstring text itself.
6. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
7. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# Output requirements
- Output only the revised Google-style docstring.
- Do not add explanations, commentary, or surrounding prose.
- Preserve any correct existing wording that is supported by the input while filling gaps needed for a complete docstring.