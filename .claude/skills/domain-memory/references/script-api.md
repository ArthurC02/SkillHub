# File-backed Domain Memory API

`scripts/registry_tools.py` is the stable command entry point. It implements the read, query, and controlled update capability that a remote Domain Memory API would provide, using JSON files in the local repository.

| Capability | Command | Machine guarantee |
| --- | --- | --- |
| Map repository sources | `discover-sources --repo-root <repo> [--output <file>]` | Inventories instruction files and candidate decision, requirement, contract, boundary, implementation, and test paths without inferring domain facts. |
| Assess project readiness | `readiness --repo-root <repo> [--registry-root <path>]` | Reports the posture described in [readiness](readiness.md). It is read-only and never promotes a fact or selects a source corpus. |
| Initialize from human choices | `init-domain-memory --repo-root <repo> --output <path> --source <path> --storage-mode <mode> --data-classification <class> --review-mode <mode> --source-authority <identity> [--include <glob>] [--exclude <glob>]` | Requires an explicit destination and source choices. Applies include/exclude and policy resource limits before writing the confirmed map. |
| Verify selected sources | `verify-sources --repo-root <repo> --source-map <path> [--policy <path>]` | Compares each selected source to its initialization snapshot and, when supplied, rechecks policy limits. |
| Focus selected sources | `refine-sources --registry-root <root> --repo-root <repo> --source <path>...` | Replaces a broad source corpus with focused paths, rebuilds snapshots, and resets selection to agent-asserted until a developer confirms it. |
| Verify citations | `verify-evidence --registry-root <path> --repo-root <repo>` | Detects missing, malformed, and changed structured evidence citations. |
| Upgrade legacy citations | `migrate-evidence --registry-root <path> --repo-root <repo>` | Converts only existing, in-repository `path:line` citations into digest-backed structured citations through a journaled update. |
| Verify audit trail | `verify-audit --registry-root <path>` | Validates the local append-only hash chain and its small head manifest; it is tamper-evident, not an external immutable log. |
| Recover an interrupted update | `recover-registry-update --registry-root <path> [--force]` | Uses the phase journal and matching audit operation to roll back or complete a transaction; `--force` is required only after confirming that a crashed writer left its lock behind. |
| Report Registry health | `probe --repo-root <repo> --registry-root <path> [--policy <path>]` | Says whether a Domain Memory is present, current, and how its sources were established. It reports posture, not whether a record is true. |
| Validate the Registry | `validate --registry-root <path> [--repo-root <repo>] [--require-reviewed]` | Checks file shape, asset formats, status values, IDs, and references. `--require-reviewed` additionally refuses a Registry whose records are candidates or whose policy cannot establish a reviewed record at all. |
| Report modelling gaps | `coverage --registry-root <path>` | Lists Contexts holding no vocabulary, aggregate, rule, or contract, separating an examined absence from an unexamined one. |
| Build an evidence citation | `cite --repo-root <repo> --path <file> --start <line> --end <line> [--registry-root <path>]` | Produces the digest-backed citation object a record's `evidence` requires. Build citations with this rather than by hand. |
| Retrieve a record | `get-record --asset <asset> --id <id>` | Returns the exact stored record or fails. |
| Resolve vocabulary | `resolve-terms --query <text> [--context <id>]` | Matches a Vocabulary record when its name or id appears in the query, or the query appears in its name, id, or definition; a whole sentence is a valid query. No match does not prove absence. |
| Confirm the selected sources | `confirm-sources --registry-root <root> --repo-root <repo> --confirmed-by <identity>` | Moves the source map from `agent-asserted` to `developer-confirmed`, recording who and when in the audit chain. Refused without a named developer, and refused once the sources have moved. |
| Retrieve a Context model | `get-context --id <id>` | Returns the Context with related vocabulary, aggregates, rules, contracts, and interactions. |
| Inspect a boundary | `analyze-boundary --source-context <id> --target-context <id>` | Lists registered direct interactions and contracts. |
| Search an asset | `lookup --asset <asset> --query <text>` | Performs deterministic file-backed text matching. |
| Add or revise a candidate | `upsert-candidate --registry-root <path> --repo-root <repo> --asset <asset> --record-file <file>` | Validates a staged complete Registry and applies a recoverable update. |
| Apply an approved change | `apply-approved-updates --package-root <path> --registry-root <path> --repo-root <repo>` | Requires a valid approved current Change Package and promotes only new or candidate records through a recoverable update. |
| Start a Change Package | `init-change-package --output <path>` | Writes the requirement, proposal, obligation, and evidence templates a material change is reviewed through. |
| Validate a Change Package | `validate-change-package --package-root <path> --registry-root <path>` | Requires an obligation per acceptance criterion, rule, and contract, a named counterfactual check, and updates that reference existing assets. |
| Submit for review | `submit-proposal --package-root <path> --registry-root <path> --repo-root <repo>` | Captures the exact Git HEAD and Registry digest before changing a valid draft to `submitted`. |
| Retire a proposal | `supersede-proposal --package-root <path> --reason <text> [--superseded-by <id>]` | Keeps the status the proposal died in, its reason, and its replacement, rather than editing it back to a draft. |
| Record human approval | `record-approval --package-root <path> --role <role> --reviewer <identity> --scope <scope>` | Rejects self-approval and duplicate approval for the same role and revision. |
| Verify test attestations | `verify-proposal --package-root <path> --registry-root <root> --repo-root <repo>` | Promotes only a submitted Proposal that has a passing, digest-backed result for every obligation. |
| Finalize approval | `finalize-proposal --package-root <path> --registry-root <root> --repo-root <repo>` | Promotes only a verified Proposal whose required approval roles, externally verifiable SCM evidence, traceability, and base revision still validate. |

## Preparing the approval authority

A Registry initialized as `local-draft-only` produces candidates and nothing else; no command promotes a record until the policy names an authority that can be checked from outside the package. Two verifiers cover two situations, and `discover-sources` recommends the one that fits the repository.

A repository whose pull requests are reviewed uses `github-pr`: the merged, approved, green pull request is the evidence, and `finalize-proposal` confirms it through the provider API. This needs a reviewer other than the proposal's author, because a hosting provider does not let an author approve their own pull request.

A repository without that reviewer uses `git-signed-commit`: the maintainer signs the commit that carries the change, and the signature is the evidence.

| Capability | Command | Machine guarantee |
| --- | --- | --- |
| Report what governance still needs | `governance-readiness --registry-root <path> --repo-root <repo>` | Names the missing pieces: no authorized signer, or no pre-push hook on this machine. A `local-draft-only` Registry reports working memory and needs nothing. |
| Prepare a signing identity | `init-signing-key --repo-root <repo> --principal <identity> [--key-file <path>] [--force] [--sign-every-commit]` | Creates or adopts an ed25519 key, writes the allowed-signers file the verifier reads, points Git at both, and reports the principal and fingerprint to authorize. It prints no private key material. When `DOMAIN_MEMORY_SIGNING_KEY` holds an OpenSSH private key it adopts that instead of generating one, so a container receives the identity as a secret; material it cannot derive a public key from is refused and the partial file removed. |
| Change one governance field | `amend-policy --registry-root <path> --field <field> --value <value> --reason <text> [--verifier <verifier>]` | Refuses a policy the validator rejects before writing, and appends the old value, new value, and reason to the audit chain. Leaving `local-draft-only` requires `--verifier`, because a review mode and the authority behind it move together. `authorized_signers` takes a comma-separated list, so a lost key can be rotated out. Selected source paths and limits are settled at initialization and are not amendable. |
| Enforce signatures on push | `install-git-hitl-hook --registry-root <path> --repo-root <repo>` | Installs a pre-push hook in the directory Git actually runs, preserving any existing hook beside it and running that first. It checks every pushed commit that touches the Registry. A local hook is bypassable with `--no-verify` and absent on a machine that never installed it, so it is an early warning, not the trust boundary. |
| Check one commit's signature | `verify-git-governance --registry-root <path> --repo-root <repo> --commit <sha>` | Requires a valid signature whose principal or key fingerprint is authorized by the policy, and requires the commit to change Registry files. |
| Check a policy before adopting it | `validate-policy --policy <file>` | Reports every reason a policy would be refused, including a review mode and verifier that contradict each other. |
| Check SCM evidence alone | `verify-scm-attestation --attestation <file> --package-root <path>` | Checks a single attestation against its proposal without finalizing anything. |
| Withdraw local reviewed status | `demote-local-reviews --registry-root <path>` | Returns every reviewed record to candidate and strips its review metadata. Only a `local-draft-only` Registry accepts it: it is how a Registry retracts approvals that turned out not to be externally verifiable. |

Authorize a key by its fingerprint rather than its principal when the address it was created under may change. Both are reported by `init-signing-key` and either satisfies the check.

All command output is JSON except success or error messages. Registry-dependent commands require `--registry-root`; `readiness` may omit it when assessing a repository before Domain Memory exists. The scripts do not grant authorization: call a write command only when the current user and repository process authorize it.

For an update, include `registry_updates` in `domain-change-proposal.json`. Each update uses `operation: "upsert"`, names an asset, and contains the complete record. Submission records a deterministic digest of all standard Registry files plus the observed Git HEAD when available. Finalize and apply require the Registry digest to remain current; an unrelated Git commit alone does not invalidate review. They do not traverse Git history. `verify-proposal` validates submitted test results as attestations; it does not run an arbitrary command from the package. The apply command validates the Change Package, then rejects an update that would overwrite a reviewed record. Create a new record or an explicitly governed superseding proposal for a reviewed change.
