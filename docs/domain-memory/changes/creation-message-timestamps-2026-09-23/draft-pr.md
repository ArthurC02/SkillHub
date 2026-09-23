# Draft PR: Timestamp each interactive-creation message

## Business intent

Creators can see when every persisted conversation message was recorded.

## Domain impact

Skill Creation owns the timestamp with the message. The public snapshot gains an optional field; the model-provider request remains unchanged.

## Implementation handoff

No new tactical pattern is needed. One owner-local append operation records content, role and UTC time together. A parallel collection is rejected because it can silently drift; provider metadata is rejected because it would enter model context. The proof covers visible timestamps, legacy absence and provider exclusion; mutation removes timestamp assignment and must fail the focused test.

## Proposal and approvals

`creation-message-timestamps-2026-09-23` is a draft material proposal requiring developer review before Registry promotion. There are no unresolved product questions.

## Contract impact

The public CreationMessage response adds an optional RFC 3339 timestamp. It is backward compatible and introduces no event or model-provider contract change.

## Verification

Pending implementation.

## Residual risks

Existing persisted messages have no historical timestamp and render without one.
