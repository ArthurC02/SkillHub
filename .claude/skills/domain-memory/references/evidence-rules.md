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

Every citation has one of these kinds, derived from the confirmed source map whenever it is checked rather than declared by an author or stored:

- The most specific selected path containing the cited file decides, so a curated boundary file keeps its kind even when the directory above it is also a selected source.
- A citation inside a selected path that no discovered group classifies, such as a migrations directory, is `selected`: the developer chose it, so it counts as a confirmed source.
- A citation outside every selected path is recorded as `unclassified`: the confirmed corpus does not cover it, not that the citation is wrong.
- `verify-evidence` counts results by kind, so a reviewer can see when a conclusion rests only on implementation.

Build every citation with `cite`. It records a cited-line digest, which is what makes the citation checkable. The line numbers locate the excerpt; the cited-line digest is what it says.

- When the cited lines no longer hold the excerpt but it occurs exactly once elsewhere in the file, because lines were added or removed above it, `verify-evidence` reports it `shifted` with the lines where it now sits; the fact still holds, so this does not fail and needs no Change Package of its own.
- Re-cite a shifted citation when a package next touches the record: `relocate-evidence --from <file> --to <file>` re-cites every shifted citation of the file, or of a directory, or of the whole repository with `.`. This holds even when the file shrank below the cited range.
- `verify-evidence --strict` also fails on a `shifted` citation, so after a re-citation package is applied it shows that none is left.
- Before moving a block out of a file, compare the block with each citation's `cited_lines`: a citation inside the block will be found in the new file by `relocate-evidence --from <old file> --to <new file>`, and one outside it stays where it is, at most `shifted`.
- Run `verify-evidence --path` on the files a change edits, before committing, to see whether any citation needs `recite`.
- A stale citation is not re-cited automatically: its text changed, so someone checks that the fact still holds before `recite` points it at new lines.
- An excerpt that changed, or that now occurs more than once, is `stale`; unrelated changes elsewhere in the file do not invalidate the cited fact. A citation assembled by hand with a different line-splitting rule verifies as `stale`.
- A citation with no cited-line digest at all verifies as `unhashed`, which `validate` refuses; `upsert-candidate` computes the digest the way `cite` does for any citation that gives a path and lines but no digest, and keeps a digest it is given.

For every conclusion, cite the path and enough location information for a reviewer to reproduce it. When durable provenance matters, record the repository revision and whether each cited source has uncommitted changes. A developer-confirmed source map also records a content digest for each selected path; run `verify-sources` before relying on an old snapshot.

When sources conflict, list each source, the precise disagreement, and the behavior that cannot be chosen safely. Prefer no source merely because it is easier to search. Superseded decisions remain historical evidence and must not be used as current rules.

A file written to instruct coding agents is not a domain source, whatever it is named and however complete its picture of the repository looks. It records how people want work done, is rewritten whenever that changes, and is the first thing a repository reorganizes away.

A file is not authoritative because of where it sits or what it is called. A repository-specific document may be read as evidence, but a Domain Memory must not come to depend on one: prefer a citation that still resolves after that document is deleted, and record the gap when none exists. Do not turn source ownership into a claim about a human approver. A repository map can identify a technical owner without establishing an organization role, approval authority, or effective period.
