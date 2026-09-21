# Project readiness

`readiness` is a read-only routing check for a repository that may not have a
Domain Memory yet. It observes candidate source locations and, when the caller
provides `--registry-root`, the source map status. It does not select sources,
create a Registry, promote a record, or infer a domain owner.

## States

| State | Evidence | Next capability |
| --- | --- | --- |
| `empty` | No decisions, requirements, contracts, bounded-context files, implementation, or tests are discoverable. | `discover` |
| `greenfield` | At least one design or product source is present, or a valid Registry contains domain records, but no implementation source is discoverable. | `design` |
| `brownfield` | An implementation source is discoverable. Existing Registry facts still require the normal review checks. | `read-and-maintain` |
| `dead` | A supplied Registry or source map is invalid, unverified, or stale, and no current product source is discoverable. | `recover-or-reconfirm` |

The state is a posture, not a verdict about project ownership, quality, or
whether the project is commercially active. Missing tests do not make a
project dead. A repository with an unusual layout may be reported as empty or
greenfield; run `discover-sources` and let a developer choose the corpus.

## Output contract

```json
{
  "format": "domain-memory-readiness/v1",
  "state": "empty",
  "confidence": "high",
  "signals": [{"kind": "implementation", "status": "missing"}],
  "blocks": [],
  "next_capability": "discover"
}
```

`signals` reports observations, including Registry validation and record count;
`blocks` reports reasons that the existing memory cannot safely guide work, and
`next_capability` only routes the Agent. An unusable Registry overrides the
normal design or maintenance route until its sources or structure are repaired.
An Agent must still inspect the selected sources and run the capability's own
validation before relying on any domain fact.
