# Domain Memory reliability target architecture

This is the target architecture for a public release. It makes the Registry a reviewable projection, not an authority that can invent domain facts. It separates trusted control, untrusted evidence, and verified state.

The current local implementation provides policy-controlled source selection and limits, Registry digests, journaled local writes, structured citation verification, test-attestation checks, a tamper-evident local audit chain, and SCM-artifact binding checks. It does **not** yet provide a provider API adapter, CI-run retrieval, an immutable remote audit store, a Merkle per-file source manifest, or general contract-diff adapters. Treat the sections below as requirements for those future adapters, never as a claim that they already exist.

## Trust boundaries

| Input | Treatment | Permitted effect |
| --- | --- | --- |
| Repository instructions at the agent's configured instruction path | Trusted control | Select the agent procedure. |
| Developer-confirmed source policy | Trusted configuration after review | Select readable source paths and storage location. |
| Selected documents, code, test output, issues, and generated text | Untrusted evidence | Support a cited candidate; never issue commands, select tools, approve a proposal, or grant access. |
| Script output and model output | Untrusted proposal | Require schema validation and an authorized write path. |
| SCM review, protected-branch result, and CI attestation | External verification | Satisfy the configured approval or check policy. |

The Script must not execute text from an evidence source. It must use argument arrays for commands, resolve paths inside the repository, restrict writes to the configured Domain Memory root, redact secrets from persisted output, and require explicit human authorization for destructive or external actions.

## Durable artifacts

`domain-memory-policy.json` is created during Init from developer choices. It contains the destination, selected source roots, include and exclude patterns, source authority, data classification, owners, storage mode, approved command profiles, review provider, and resource limits. Discovery only suggests values for this policy.

The Registry contains records and a canonical manifest. Each Registry revision is a canonical SHA-256 digest of every standard asset. Canonical JSON uses UTF-8, sorted keys, normalized line endings, and an explicit schema version so formatting changes do not invalidate a review.

Every reviewed evidence citation is an object, not only `path:line`:

```json
{
  "path": "docs/rules.md",
  "lines": {"start": 12, "end": 16},
  "content_sha256": "sha256:<file-content>",
  "excerpt_sha256": "sha256:<cited-lines>",
  "observed_commit": "<40-hex-sha-or-null>",
  "observed_at": "<ISO-8601>"
}
```

The source manifest is a Merkle-style list of selected files and their digests. It records skipped files and the reason. `verify-evidence` identifies stale records precisely; a changed source root alone does not silently invalidate unrelated evidence.

## State model

Record maturity and Change Package workflow are separate:

| State | Meaning |
| --- | --- |
| `candidate` | Evidence or a proposed interpretation exists; it cannot constrain implementation. |
| `reviewed` | A configured authority accepted a specific record revision and evidence manifest. |
| `deprecated` | Retained for historical lookup but excluded from current decisions. |
| `superseded` | Replaced by a named record or Change Package. |

| Proposal state | Required proof |
| --- | --- |
| `draft` | Valid schema and explicit unknowns. |
| `submitted` | Frozen proposal digest, Registry digest, source manifest digest, and test plan. |
| `verified` | CI attestation for the submitted digest. |
| `approved` | Review-provider evidence for that exact proposal and commit. |
| `applied` | A committed Registry revision and update attestation. |
| `rejected` or `superseded` | Immutable terminal record. |

Changing proposal content, Registry content, cited evidence, or test configuration creates a new digest and invalidates `verified` and `approved`.

## Update protocol

1. Read the current Registry manifest and source manifest once.
2. Build a complete proposed Registry in a temporary workspace.
3. Run schema, reference, semantic policy, evidence, contract, and test-plan validation there.
4. Acquire a per-Registry writer lock.
5. Re-read the Registry manifest under that lock. If it differs from the expected digest, return `conflict` with no write.
6. Apply the prepared patch through one recoverable transaction, then emit an update attestation containing old digest, new digest, proposal digest, and actor identity.
7. Commit the patch through the repository's normal SCM workflow. Mark the proposal `applied` only after observing the committed revision or accepted merge result.

The lock protects local writer races. Git is the collaboration and audit boundary: the Skill prepares a change, while protected branches, code owners, required checks, and stale-review invalidation decide whether it becomes shared truth.

## Git use and bounded history

Normal read, validation, and update operations use only the current worktree, canonical Registry digest, and optional `git rev-parse HEAD`. They never invoke `git log`, ancestry walks, or an unbounded `git diff`.

If a future `history` command is added, it must require an explicit revision range and enforce maximum commits, bytes, duration, and a visited-SHA set. History is an explanatory query only; correctness must not depend on traversing it.

The HEAD SHA is provenance metadata. It must not by itself invalidate a Proposal when the Registry digest and all cited evidence digests are unchanged; unrelated commits should not force needless reapproval.

## Approval and test attestations

The default local mode can create drafts and candidate records but cannot establish a reviewed record. Promotion requires a configured verification adapter. A Git hosting adapter verifies the pull request, target commit, required reviewer roles, code-owner requirements, and completed checks through the provider API. It stores references and immutable observed fields, never a claimed reviewer name alone.

Test attestations use an allowlisted command profile from the policy. They record command ID, arguments, working directory, environment profile name, input and output digests, exit code, skipped count, duration, commit, and CI run URL when available. Free-text `passed` is never sufficient proof.

Contract adapters validate the declared format and produce structural diffs. A breaking diff creates a required decision; it cannot decide business compatibility by itself.

## Reliability, privacy, and scale limits

All parsers reject unknown schema versions, duplicate identifiers after Unicode normalization, path traversal, malformed journals, oversized JSON, and non-regular files. Recovery journals use fixed operation IDs and paths derived by the Script, never arbitrary paths read from the journal.

Init asks whether Domain Memory is tracked, ignored, or stored externally, plus its sensitivity and retention policy. Source text is not copied by default. Secret scanning runs before persistence, and findings are reported without emitting secret values.

The policy sets limits for file count, file size, total bytes, hash duration, query result count, and history traversal. Exceeding a limit produces a partial, explicitly incomplete result; it never silently omits material.

## Rollout

1. Introduce policy and canonical manifest in compatibility mode; existing Registries remain readable as `legacy-unverified`.
2. Add record-level evidence objects, source manifests, and `verify-evidence`.
3. Replace local claimed approvals and free-text test results with provider and CI attestations.
4. Move reviewed-state updates to the compare-and-swap protocol and emit `applied` attestations.
5. Add contract adapters, migration tooling, adversarial prompt-injection fixtures, and release compatibility tests.

## Policy is part of the revision

The Registry digest covers the policy as well as the assets, because the policy decides what the Registry may become: whether a record can ever be reviewed, who the sources answer to, and which command profiles a test attestation may cite. A captured base revision therefore goes stale when the policy changes, however it changed, and approvals taken against it must be sought again.

`amend-policy` changes one governance field, refuses a policy the validator rejects before writing anything, and appends the old value, the new value and a required reason to the audit chain. Selected source paths and limits are not amendable: they are settled at initialization, where a developer confirms them.
