---
name: python-docstring-google-reviewer
description: Review a Python function’s docstring, fill in missing content, and rewrite it in Google style. Use when you have a Python function and need a clean, paste-ready Google-style docstring without changing the function body.
---

# Python docstring Google reviewer

## Purpose
Review the docstring for a Python function, fill in missing docstring content, and rewrite it in Google style.

Use this skill when the user provides a Python function and wants the docstring checked or completed in Google style.

## Inputs
- The Python function source, including any existing docstring.
- Any surrounding context needed to understand parameter meaning, return value, and side effects.

## Output
Return the function with a Google-style docstring that is ready to paste back into the source code.

## Procedure
1. Read the function signature and body.
2. Inspect any existing docstring.
3. Preserve the function’s meaning and behavior.
4. Rewrite or complete the docstring in Google style.
5. Include only the sections that the function actually needs.
6. If a section is not needed, omit it.
7. Do not change the function code.

## Google-style content rules
- Start with a short summary line.
- Add `Args:` for parameters that need explanation.
- Add `Returns:` when the function returns a value.
- Add `Raises:` only if the function clearly raises exceptions that matter to the caller.
- Keep descriptions concise, specific, and aligned with the code.
- Use consistent indentation and Google-style formatting.

## Constraints
- Do not invent behavior that is not supported by the function body or supplied context.
- Do not rewrite the function logic.
- Do not add unrelated commentary.
- If the supplied code is insufficient to determine a required docstring detail, state that the missing detail cannot be inferred from the input.

## Final response shape
Provide the updated function or docstring text in a paste-ready form.