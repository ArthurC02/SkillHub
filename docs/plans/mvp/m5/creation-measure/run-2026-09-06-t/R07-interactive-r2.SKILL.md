---
name: python-docstring-google-filler
description: Review Python function docstrings, fill in missing parts, and output a Google-style docstring. Use this when you have a Python function and an existing or incomplete docstring that needs completion.
---

# Purpose
Turn an incomplete Python function docstring into a complete Google-style docstring.

# When to use this skill
Use this skill when the input is a Python function and its existing docstring needs missing content filled in, while keeping the result in Google style.

# Instructions
1. Read the Python function and its current docstring, if any.
2. Keep any correct existing docstring content.
3. Add missing Google-style sections that the function supports, based only on the input.
4. If the input gives parameters, document them in an `Args:` section.
5. If the input gives a return value, document it in a `Returns:` section.
6. If the input gives raised exceptions, document them in a `Raises:` section.
7. Preserve the meaning of the input and do not invent behavior, types, names, or details.
8. Output the completed docstring itself only.

# Required rules
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# Output requirements
- Produce a Google-style docstring.
- Keep the output focused on the docstring content for the provided function.
- If the input is silent about a detail needed for the docstring, leave that detail out rather than guessing.
- Return the finished docstring, not commentary about it.