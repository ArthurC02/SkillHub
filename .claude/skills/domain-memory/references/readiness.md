# Project readiness

`readiness` is a read-only routing check for a repository that may not have a Domain Memory yet. It observes candidate source locations and the source map status of the Registry at `--registry-root`; without that option it looks for the Domain Memories the repository holds, assesses the one it finds and names it in `registry_root` and on stderr, or names several in `domain_memories` and assesses none until the caller chooses. It does not select sources, create a Registry, promote a record, or infer a domain owner.

## States

| State | Evidence | `next_capability` | What to open |
| --- | --- | --- | --- |
| `empty` | No decisions, requirements, contracts, bounded-context files, implementation, or tests are discoverable. | `discover` | No Skill: `discover-sources`, then `init-domain-memory` with the developer's choices, as [the workflow](seven-step-workflow.md#before-step-0-map-repository-sources) says |
| `greenfield` | At least one design or product source is present, or a valid Registry contains domain records, but no implementation source is discoverable. | `design` | [domain-memory-design](../skills/domain-memory-design/SKILL.md) |
| `brownfield` | An implementation source is discoverable. Existing Registry facts still require the normal review checks. | `read-and-maintain`, or `extract` when the supplied Registry holds no record yet | [domain-memory-read](../skills/domain-memory-read/SKILL.md), then [domain-memory-maintain](../skills/domain-memory-maintain/SKILL.md) after the change; [domain-memory-extract](../skills/domain-memory-extract/SKILL.md) for `extract` |
| `dead` | A supplied Registry or source map is invalid, unverified, or stale, and no current product source is discoverable. | `recover-or-reconfirm` | No Skill: `recover-registry-update` for an interrupted write, `confirm-sources` or `refresh-sources` for a source map that is unverified or stale, as `blocks` names them |

The state is a posture, not a verdict about project ownership, quality, or whether the project is commercially active. Missing tests do not make a project dead. A repository with an unusual layout may be reported as empty or greenfield; run `discover-sources` and let a developer choose the corpus.

## Output contract

```json
{
  "format": "domain-memory-readiness/v1",
  "state": "empty",
  "confidence": "high",
  "signals": [{"kind": "implementation", "status": "missing"}],
  "blocks": [],
  "next_capability": "discover",
  "registry_root": null,
  "domain_memories": []
}
```

`registry_root` is the Registry assessed, relative to the repository, or null; `domain_memories`, present only when `--registry-root` was omitted, lists every Domain Memory the repository holds.

`signals` reports observations, including Registry validation, record count and, with a usable Registry, a `listing` signal that is `current` or `drifted` with the paths `refresh-listing` would add and drop; `blocks` reports reasons the existing memory cannot safely guide work; `next_capability` only routes the Agent. An unusable Registry overrides the normal design or maintenance route until its sources or structure are repaired. An Agent must still inspect the selected sources and run the capability's own validation before relying on any domain fact.
