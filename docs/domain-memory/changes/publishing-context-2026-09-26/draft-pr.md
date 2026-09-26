# Draft PR: Publishing becomes its own Bounded Context

## Business intent

A creator discloses a Skill under a publisher name of their own, at a public address, without the catalog vouching for it.

## Domain impact

New core context `publishing` owns publisher names, publications and immutable releases. It may query Identity for the caller's workspace; every other context is denied in both directions. Registry facts arrive through injected functions only.

## Implementation handoff

A release is an append-only row pinning version and content hash. The public read re-checks the pinned Skill's takedown, hold and redistribution verdict on every read. Rejected: a published flag on the Skill, and copying the verdict into the release.

## Proposal and approvals

Proposal `registry-publishing-context`; one developer approval.

## Contract impact

New public endpoints for the publisher, the owner's publication and the public address; no existing contract changes.

## Verification

Name rule, release gate and availability unit tests; integration tests for the public address, republishing, ownership and account purge; the immutability SQL test; automation-check for the import guard.

## Residual risks

The public address does not hand out a package yet, and a publication is never listed in the catalog until an exposure review exists.
