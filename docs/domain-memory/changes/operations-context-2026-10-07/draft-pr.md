# Draft PR: Operations becomes the home of the platform's agents

## Business intent

Operators decide which of the platform's own agents may run, and can stop one agent or all of them before their next step.

## Domain impact

New supporting context `operations` owns agent registration, the global agent brake and agent run records. It imports no other context; the operator's identity reaches its handler through a function the composition root injects. Every other context is denied in both directions.

## Implementation handoff

Agent definitions live in code and are mirrored into the database at worker start, disabled until an operator enables them. A run checks its agent and the brake before every step and ends as stopped when either says so. Rejected: agents defined only in the database, because their tools and actions are Go registries a row cannot extend.

## Proposal and approvals

Proposal `registry-operations-context`; one developer approval.

## Contract impact

New operator endpoints to list agents, enable or disable one, and engage or release the brake; no existing contract changes.

## Verification

Halt-reason unit test; integration tests for disabling, the brake, operator-only access and note validation; automation-check for the import guard.

## Residual risks

No agent runs yet: the run loop, the daily report agent and the operator page come with the next work items.
