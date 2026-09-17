# File-backed Domain Memory API

`scripts/registry_tools.py` is the stable command entry point. It implements the read, query, and controlled update capability that a remote Domain Memory API would provide, using JSON files in the local repository.

| Capability | Command | Machine guarantee |
| --- | --- | --- |
| Map repository sources | `discover-sources --repo-root <repo> [--output <file>]` | Inventories instruction files and candidate decision, requirement, contract, boundary, implementation, and test paths without inferring domain facts. |
| Initialize from human choices | `init-domain-memory --repo-root <repo> --output <path> --source <path> --storage-mode <mode> --data-classification <class> --review-mode <mode> --source-authority <identity> [--include <glob>] [--exclude <glob>]` | Requires an explicit destination and source choices. Applies include/exclude and policy resource limits before writing the confirmed map. |
| Verify selected sources | `verify-sources --repo-root <repo> --source-map <path> [--policy <path>]` | Compares each selected source to its initialization snapshot and, when supplied, rechecks policy limits. |
| Verify citations | `verify-evidence --registry-root <path> --repo-root <repo>` | Detects missing, malformed, and changed structured evidence citations. |
| Upgrade legacy citations | `migrate-evidence --registry-root <path> --repo-root <repo>` | Converts only existing, in-repository `path:line` citations into digest-backed structured citations through a journaled update. |
| Verify audit trail | `verify-audit --registry-root <path>` | Validates the local append-only hash chain and its small head manifest; it is tamper-evident, not an external immutable log. |
| Recover an interrupted update | `recover-registry-update --registry-root <path> [--force]` | Uses the phase journal and matching audit operation to roll back or complete a transaction; `--force` is required only after confirming that a crashed writer left its lock behind. |
| Retrieve a record | `get-record --asset <asset> --id <id>` | Returns the exact stored record or fails. |
| Resolve vocabulary | `resolve-terms --query <text> [--context <id>]` | Matches a Vocabulary record when its name or id appears in the query, or the query appears in its name, id, or definition; a whole sentence is a valid query. No match does not prove absence. |
| Confirm the selected sources | `confirm-sources --registry-root <root> --repo-root <repo> --confirmed-by <identity>` | Moves the source map from `agent-asserted` to `developer-confirmed`, recording who and when in the audit chain. Refused without a named developer, and refused once the sources have moved. |
| Retrieve a Context model | `get-context --id <id>` | Returns the Context with related vocabulary, aggregates, rules, contracts, and interactions. |
| Inspect a boundary | `analyze-boundary --source-context <id> --target-context <id>` | Lists registered direct interactions and contracts. |
| Search an asset | `lookup --asset <asset> --query <text>` | Performs deterministic file-backed text matching. |
| Add or revise a candidate | `upsert-candidate --registry-root <path> --repo-root <repo> --asset <asset> --record-file <file>` | Validates a staged complete Registry and applies a recoverable update. |
| Apply an approved change | `apply-approved-updates --package-root <path> --registry-root <path> --repo-root <repo>` | Requires a valid approved current Change Package and promotes only new or candidate records through a recoverable update. |
| Submit for review | `submit-proposal --package-root <path> --registry-root <path> --repo-root <repo>` | Captures the exact Git HEAD and Registry digest before changing a valid draft to `submitted`. |
| Record human approval | `record-approval --package-root <path> --role <role> --reviewer <identity> --scope <scope>` | Rejects self-approval and duplicate approval for the same role and revision. |
| Verify test attestations | `verify-proposal --package-root <path> --registry-root <root> --repo-root <repo>` | Promotes only a submitted Proposal that has a passing, digest-backed result for every obligation. |
| Finalize approval | `finalize-proposal --package-root <path> --registry-root <root> --repo-root <repo>` | Promotes only a verified Proposal whose required approval roles, SCM attestation, traceability, and base revision still validate. |

All command output is JSON except success or error messages. Pass `--registry-root` to every read or update command. The scripts do not grant authorization: call a write command only when the current user and repository process authorize it.

For an update, include `registry_updates` in `domain-change-proposal.json`. Each update uses `operation: "upsert"`, names an asset, and contains the complete record. Submission records a deterministic digest of all standard Registry files plus the observed Git HEAD when available. Finalize and apply require the Registry digest to remain current; an unrelated Git commit alone does not invalidate review. They do not traverse Git history. `verify-proposal` validates submitted test results as attestations; it does not run an arbitrary command from the package. The apply command validates the Change Package, then rejects an update that would overwrite a reviewed record. Create a new record or an explicitly governed superseding proposal for a reviewed change.
