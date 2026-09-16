# Evidence rules

Classify each source before drawing a conclusion.

| Source kind | Establishes | Must still be checked against |
| --- | --- | --- |
| Accepted decision or architecture rule | Current intended constraint | Its supersession status and implementation where behavior matters |
| Requirement and acceptance criterion | Intended observable outcome | Current decision and implementation constraints |
| Public or event contract | Inter-process promise | Version, generated artifacts, and producer or consumer behavior |
| Implementation and database constraint | Current executable behavior | Whether it is intentional and covered by acceptance criteria |
| Test or CI result | Evidence that a specific check ran | Scope, skips, test inputs, and other required checks |
| Uncommitted file or working-tree change | A candidate proposal | Its authoring, review, and authority |

A stored citation records which of these kinds it came from. `upsert-candidate` and `migrate-evidence` derive that kind from the confirmed source map rather than asking an author to declare it: the most specific selected path containing the cited file decides, so a curated boundary file keeps its kind even when the directory above it is also a selected source. A citation outside every selected path is recorded as `unclassified`, which means the confirmed corpus does not cover it, not that the citation is wrong. `verify-evidence` counts results by kind, so a reviewer can see when a conclusion rests only on implementation.

For every conclusion, cite the path and enough location information for a reviewer to reproduce it. When durable provenance matters, record the repository revision and whether each cited source has uncommitted changes. A developer-confirmed source map also records a content digest for each selected path; run `verify-sources` before relying on an old snapshot.

When sources conflict, list each source, the precise disagreement, and the behavior that cannot be chosen safely. Prefer no source merely because it is easier to search. Superseded decisions remain historical evidence and must not be used as current rules.

A file is not authoritative because of where it sits or what it is called. A repository-specific document may be read as evidence, but a Domain Memory must not come to depend on one: prefer a citation that still resolves after that document is deleted, and record the gap when no such citation exists. Do not turn source ownership into a claim about a human approver. A repository map can identify a technical owner without establishing an organization role, approval authority, or effective period.
