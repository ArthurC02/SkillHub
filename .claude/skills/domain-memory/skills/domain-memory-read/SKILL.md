---
name: domain-memory-read
description: Read the reviewed Domain Registry before coding. Use when a request names a domain term or may cross a Context; it answers who owns what, which invariants hold, and what must commit together, and writes nothing.
---

# Read Domain Memory

Use this before coding when a request uses a domain term or may cross a Context.

1. In an unfamiliar repository, run `readiness --repo-root <repo> [--registry-root <root>]` first ([readiness](../../references/readiness.md) explains its states); without `--registry-root` it cannot report `extract` or `dead`. It is read-only and routes only: `empty` and `greenfield` need source discovery or Design before any Registry read, and `dead` needs recovery. Do not call `probe` as the first command.
2. Run `probe --repo-root <repo> --registry-root <root>`.
3. Run `validate --registry-root <root> --repo-root <repo>`; it also checks the policy beside the Registry.
4. Resolve terms with `resolve-terms`, then inspect the owner with `get-context` and `get-record`; before changing a record, `dependents` lists what else the change may touch. When the work reads or writes a database table, `describe-table --migrations <dir> --table <name>` gives that one table's meaning, each column's meaning and allowed values, its constraints and relations, and the records that mention it, in place of reading the migrations or guessing from column names.
5. Inspect existing collaboration with `analyze-boundary` before proposing a new dependency.
6. Before coding, answer these from reviewed records: which boundary keeps each invariant; what must commit together and what may arrive later; what needs identity; which step is a domain decision, which orchestration, which transport; which Context owns each fact and each policy acting on it; what stays true after failure, retry, duplicate, or partial completion; whether a pattern protects a rule or only adds indirection; whether the change leaves the process. [Tactical design reasoning](../../references/tactical-reasoning.md) explains each. Decide whether a tactical pattern is needed, then choose an idiomatic implementation only after identifying the owner, invariant, consistency boundary, and failure behavior.
7. For a material change, emit the [implementation handoff](../../references/implementation-handoff.md) with reviewed facts, forces, unknowns, and proof obligations. Do not invent the decision in Read mode when the change needs Design.

Use `reviewed` records as constraints. Candidate records are Working Memory: use them to find terms, owners, evidence, and questions to verify, but never present them as settled policy or use them to authorize a material change. Treat stale sources, unverified maps, and missing records as gaps to report. Do not write files in this mode. See [the file-backed API](../../references/script-api.md).
