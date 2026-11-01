---
type: Business Rule
status: draft
---
When the runtime cut the final output, a check that reads it records undetermined, not failed, for a value it cannot find, and undetermined, not passed, for a length within the limit; a value found in the kept part still passes and a kept part already over the limit still fails.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A cut answer that does not mention 2026-10-03 is undetermined, with the reason that the final output was cut off at 10 characters.
- A cut answer of three characters is undetermined against a three-character limit and fails against a two-character limit.
