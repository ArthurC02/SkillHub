# Reliability architecture

What the scripts guarantee about trust, writes and approval: for a host deciding what to rely on, and for an Agent asked whether a fact can be trusted. The commands themselves are in [script-api.md](script-api.md).

## Trust boundaries

| Input | Treatment | Permitted effect |
| --- | --- | --- |
| Repository instructions | Trusted control | Select the procedure. |
| The developer-confirmed source map and policy | Trusted configuration | Select the paths a citation may name, where the Registry lives, and who may approve. |
| Selected documents, code, test output and generated text | Untrusted evidence | Support a cited candidate; never issue a command, select a tool, approve a proposal or grant access. |
| Script output and model output | Untrusted proposal | Pass validation and an authorized write path before they land. |
| A merged, approved pull request with green checks, or a signed commit | External verification | Satisfy the approval the policy names. |

No command executes text from an evidence source. A test or counterfactual command runs as an argument array without a shell, every cited path resolves inside the repository, writes stay under the Registry root, and the only outside call is the pull-request check that `ship-package` and `verify-scm-attestation` make against the provider the policy names, with a token from the environment variable the command names.

## Durable artifacts

`domain-memory-policy.json` holds the choices `init-domain-memory` takes from a developer: the destination, the sources with their include and exclude patterns, the source authority, the data classification, the storage mode (`tracked`, `ignored` or `external`), the review mode with its verifier, trigger and CI requirement, the authorized signers, the command profiles a test result may run, the limits, and the `schema_conventions` the migration reader applies (exempt columns, status-like suffixes, free-text markers and the value limit), which a project edits to match its own naming. Discovery only suggests values for it. The sources and limits are settled at initialization; `amend-policy` changes the governance fields as one audited change.

A Registry revision is one SHA-256 digest over the canonical JSON of every asset and the policy. Formatting changes therefore do not invalidate a review, and a policy change does: a base revision captured before it is stale, and approvals taken against it must be sought again.

A citation is an object, `{path, lines, excerpt_sha256}`, never a bare `path:line`, so `verify-evidence` can say of each one whether the cited lines still stand. The source map holds a digest and a listing of every selected file, so `verify-sources` tells the selection moving from a file changing inside, and a changed source root alone invalidates no unrelated evidence.

## State model

Record maturity and Change Package workflow are separate states.

| Record status | Meaning |
| --- | --- |
| `candidate` | Evidence or a proposed interpretation exists; it constrains no implementation. |
| `reviewed` | The configured authority accepted this record revision and its evidence. |
| `deprecated` | Kept for lookup, excluded from current decisions. |
| `superseded` | Replaced by a named record or Change Package. |

| Proposal status | What it proves |
| --- | --- |
| `draft` | The package validates and its unknowns are stated. |
| `submitted` | The content is frozen, with the Registry revision it was built on. |
| `verified` | Every test obligation has a passing result with the digest of the run's output, from `record-test-result` or from a CI run. |
| `approved` | An approval is recorded for every role the policy and the requirement call for. |
| `applied` | The Registry is written and the audit event appended. |
| `superseded` or `rejected` | Terminal. |

A change to the proposal's content, or to the records it touches, after submission invalidates `verified` and `approved`: `rebase-proposal` restamps the base and retires the approvals.

## The write protocol

Every write goes through one routine:

1. Take the per-Registry writer lock, and finish or roll back an interrupted write first.
2. When the caller named the revision it expects, as `ship-package` does, compare it with the Registry's digest; a difference refuses with `registry revision changed before update; run ship-package again`.
3. Copy the Registry to a staging directory, apply the change there and validate the whole copy; an error the Registry did not already have refuses the write.
4. Journal the operation as `prepared`, move the Registry aside as the backup, install the staging copy, and mark the journal `installed`.
5. Append the audit event and mark the journal `audited`; then remove the backup, the staging copy and the journal. A failed append restores the backup.

`recover-registry-update` reads the journal: `prepared` restores the backup and discards the staging copy, `installed` rolls back to the backup, `audited` only cleans up, and a journal whose backup or staging directory is gone, or that is marked `reconciliation-required`, stops with `registry transaction requires manual reconciliation`. A journal of another format, malformed or oversized, is refused rather than acted on. The lock protects local writers; Git is the collaboration boundary, so protected branches, required reviews and checks guard the Registry's commit as they guard any other file.

The HEAD commit is provenance. A proposal's base is its Registry digest, so unrelated commits do not force a new approval.

## Approval and test attestations

A Registry whose review mode is `local-draft-only` makes candidates and nothing else; no command promotes a record until the policy names a verifier. `github-pr` reads the pull request through the provider's API: it must be merged, hold a current approval, have every check run completed and successful, and its head commit must carry this package with the same proposal id and revision. `git-signed-commit` checks the signature on the commit that carries the package against the authorized signers, by key fingerprint or principal. A reviewer name typed into a file is never evidence.

A test result is recorded by `record-test-result`, which runs the obligation's check under one of the policy's command profiles and records the command, the profile, exit code 0, the digest of the run's output and when it finished; a result from a CI run is entered with the same fields, the run's link as its evidence, and `verify-proposal` refuses one without the digest. A bare `passed` is never enough. `validate-contract` parses a contract schema locally and answers `unverified` for one it cannot, never `valid`.

## Limits and refusals

Every parser refuses a duplicate identifier, a path outside the repository, a journal or source map of another format, and, among the sources, a symbolic link. The policy's limits, `max_file_count`, `max_file_bytes`, `max_total_bytes` and `max_query_results`, refuse what exceeds them by name; nothing is silently truncated. `scan-secrets` reports where a secret-shaped string sits without printing it; it is a command to run before a commit, not a step a write runs for you.

## The audit log

The audit log is hash-linked as a git history is, not as a single chain: an event names the event or events it was appended on, so two clones that each append on the same base both stay valid, and the next append after their merge names both heads. A merge therefore needs no hand resolution of a hash chain: `reconcile-audit` removes git's conflict markers, keeps each event once, rewrites the manifest and joins the heads with one event. What the DAG does not give is a total order; `recorded_at` and the heights in `sequence` order events within a clone, and the join event records when two lines of work met. The asset indexes are derived from the concept files, so a merge that conflicts in an index is repaired by `refresh-indexes`, not by hand; `validate` refuses any Registry file that still holds conflict markers and any index that no longer lists the records beside it.
