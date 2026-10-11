# Draft PR: operators set each platform agent's daily spend cap

## Business intent

Operators decide how much each platform agent may spend a day without redeploying, within a ceiling the platform fixes.

## Domain impact

Operations gains one rule: an agent's effective daily spend cap is the operator's override when set, otherwise the default its definition declares. An override lies in (0, US$5]; re-registering definitions replaces only the default. No boundary moves: the agent loop reads the cap through the Journal port operations implements.

## Implementation handoff

A nullable override column beside the defined default, one query for the effective cap, the Journal supplying it at every run start, and an operator endpoint that refuses values outside the range and audits the previous and new cap in the same transaction. Rejected: overwriting the default, which the next worker start would undo; one environment variable per agent, which the admin workbench cannot see or change.

## Proposal and approvals

Proposal `operations-agent-spend-cap-override`; one developer approval.

## Contract impact

New `PUT /admin/agents/{name}/spend-cap`; the agent listing adds the defined default and whether an override is set.

## Verification

Integration tests for the bounds, re-registration, clearing, the next run's key budget and operator-only access; the bound is mutated once as the counterfactual.

## Residual risks

The sum of all agents' caps is still not checked against the gateway's global daily total (`05` R-96).
