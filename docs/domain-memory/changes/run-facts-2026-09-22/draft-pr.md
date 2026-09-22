# Promote the Run and Registry slice to reviewed facts

An earlier promotion of these same records was applied and then demoted, because
its approval was recorded inside the package (`provider: git`,
`pull_request: local-review`) rather than verified outside it. The policy now
names GitHub pull request review as the verifier, so approving and merging this
pull request is the evidence that promotion rests on.

## What becomes a reviewed constraint

| Record | Claim | Cited source |
| --- | --- | --- |
| `contexts/run` | Run Orchestration owns the Skill trial Run lifecycle | `docs/development/platform-context-map.md:24` |
| `contexts/registry` | Skill Registry owns Skill assets and version history | `docs/development/platform-context-map.md:21` |
| `aggregates/run` | `Run` is the aggregate root holding lifecycle transitions | `docs/development/platform-ddd-convergence.md:120` |
| `contracts/registry-run-facts-v1` | Registry exposes Skill and Version facts to Run as an internal fact reader | `apps/platform/internal/entrypoint/wiring/trial.go:14-30` |
| `interactions/registry-run-preflight-facts` | Run reads those facts synchronously through a composition-root reader | `apps/platform/internal/trial/execution/preflight.go:199-240` |

Each claim was re-read against its cited lines before it was proposed.

## What is deliberately left a candidate

- `vocabulary/run` defines Run as "Skill trial lifecycle", but the line it cites
  enumerates Run's commands and does not define the term.
- `rules/run-lifecycle-owner` restates the aggregate invariant word for word, and
  an ownership statement is not the falsifiable business constraint the Rule
  asset exists for.

Both need a Design decision, not a promotion. `validate --require-reviewed`
therefore still reports the Registry as not fully reviewed after this change.

## Evidence

All seven obligations carry a real command, exit code and output digest:
`validate`, `verify-evidence` (7 of 7 citations current), `verify-audit` (valid
over 17 events), `analyze-boundary` registry to run (`status: known`), and
`governance-readiness` (`ready`, no blocks). The counterfactual stripped
evidence from two reviewed records on a copy and confirmed the validator refuses
them.

## After merge

`finalize-proposal` verifies this pull request through the GitHub API (merged,
approved, checks green) and only then may `apply-approved-updates` write the
reviewed status.
