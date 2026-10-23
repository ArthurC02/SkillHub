---
type: Business Rule
status: draft
owner: run-execution
version: unknown
effective_from: unknown
effective_until: unknown
---
Every model gateway key issued for a run attempt gets the tokens-per-minute limit in force when it is issued: the operator-set value in run_rate_limit_setting, or the deployment's when none is set. The limit is not kept in the Run's snapshot, so a change reaches the next attempt of a Run already created.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A Run created before the operator sets 20000 tokens per minute has its first attempt's key issued at 20000.
- With no operator-set value, a key is issued at the deployment's limit, 200000 by default.
