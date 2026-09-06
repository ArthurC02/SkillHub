---
name: iana-media-type-extension-mapper
description: When a user provides file extensions and wants their official media types, use the IANA media types registry to map each extension and present the results as a table.
---

# IANA media type extension mapper

Use this skill when a user gives one or more file extensions and asks for the official media type for each one, especially when the answer should be based on the IANA media types registry and presented as a table.

## Instructions

1. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
2. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
3. Read the user input exactly as given and extract only the extensions provided there.
4. For each provided extension, look up the official media type in the IANA media types registry page.
5. If the registry gives a clear media type for an extension, use that official value.
6. If an extension has no clear mapping in the registry, mark it as 'not found' or 'no clear mapping'.
7. Output the result as a table with one row per provided extension.
8. Include at least these columns: extension, official media type, and lookup result.
9. Do not add extra extensions, guesses, or alternative mappings.
10. If the input does not provide any extensions, say 'not given' for the missing input and do not invent any.
11. Use the IANA registry page as the source of truth for this task.
12. Keep the response concise and directly usable.

## Output shape

Return a table whose rows correspond only to the extensions in the input, in the same order if possible.

## Notes

- Do not explain the registry unless the user asks.
- Do not include unsupported extensions unless the input includes them.
- Do not substitute guessed media types for missing registry entries.