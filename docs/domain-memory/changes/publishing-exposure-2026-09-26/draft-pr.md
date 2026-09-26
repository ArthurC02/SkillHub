# Draft PR: Publishing owns Bundles and exposure reviews

## Business intent

Only a release an operator has read enters search and the catalog, and only while readers find what the operator read. A creator can also pin several Skill versions as one Bundle.

## Domain impact

The `publishing` context widens its responsibility to Bundles and operator exposure reviews. Catalog keeps the search projection; it learns which publications are exposed, and Publishing learns what search holds for a Skill, each through a function the composition root injects. No new import in either direction.

## Implementation handoff

A review is an append-only row binding the newest release, its content hash and a digest of the searchable text, numbered per publication for compare-and-set. Public search, browse, counts and the detail read match skill, version and digest in the same query. Rejected: reviews owned by Catalog, and a listed flag written into the projection.

## Proposal and approvals

Proposal `publishing-exposure-review`; one developer approval. It supersedes the unsubmitted draft `catalog-exposure-review`, which placed reviews in Catalog.

## Contract impact

New operator endpoints for the review queue, one case and a review. The public address's exposure note turns true for an exposed release.

## Verification

Integration tests for the queue and snapshot, stale premises, a new release, the allowed-only gate, changed text and operator-only access; the immutability SQL test; automation-check.

## Residual risks

Bundles are not a search unit and are never exposed. The worker's creation reference search still reads the catalog workspaces only. Each public read makes one owner read per exposed publication.
