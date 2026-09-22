# Draft PR: The Run Aggregate cites its decision

## Business intent

A reviewed fact should stand on a decision the project made, not on a handbook describing how today's code happens to be arranged. The Run Aggregate was the last record standing on a handbook.

## Domain impact

The Run Aggregate keeps its root, its Context and its invariant. Only its evidence moves, from the convergence handbook's table row onto the decision that gave Skill, Run and Evaluation an aggregate root whose state only its own methods change.

## Implementation handoff

The cited handbook line names the file that currently holds the transition table. That is true today and says nothing about ownership; re-ordering the handbook's table would stale a reviewed record without any domain having changed. What the record actually asserts — that Run owns its lifecycle transitions — is a decision, and the decision has a line.

Chosen approach: re-cite the record through a superseding proposal, which is the only way a reviewed record's citation moves.

Rejected: leaving the handbook citation; citing the state machine listing instead, which names the transitions but not who owns them.

Counterfactual: replacing the citation's excerpt digest made verify-evidence report the source stale and exit 1.

## Proposal and approvals

`the-run-aggregate-cites-its-decision`, revision 1, superseding `run-registry-reviewed-facts-signed`. Required role: developer.

## Contract impact

None.

## Verification

Seven obligations on a staged copy. Two are specific to this change: the Run Aggregate's only citation classifies as a decision source, and no reviewed record in any asset cites a path under `docs/development`. The rest cover structural validity, citation currency, the audit chain and the external approval path.

## Residual risks

None. Forty-eight citations remain current across the Registry, split between decisions, Bounded Context documentation and implementation.
