# Draft PR: Require confirmed diagram checkpoints before Skill creation

## Business intent

Creators can turn a diagram into a Skill only after they confirm what the platform sees, answer each stated uncertainty, and confirm the completed interpretation.

## Domain impact

Creation remains the sole owner of interactive session checkpoints. A diagram-influenced draft is valid only when the newest diagram's description and fully answered interpretation were separately confirmed. The model stays behind the existing injected port; it never owns question ids, answers, or session state.

## Implementation handoff

The session snapshot gains owner-local description and interpretation values. Go assigns uncertainty ids on the second model response, persists each answer with the existing receipt and revision mechanism, and queues a follow-up only after an explicit complete-interpretation confirmation. Replacing an image invalidates all related facts. Legacy diagrams must be re-uploaded because image bytes are intentionally unavailable. The focused state tests will be mutated by removing the answer-completeness and draft-admission guards.

## Proposal and approvals

`diagram-hitl-2026-09-24` is a material draft proposal. It requires developer review before its candidate Domain Memory rule can become reviewed. No implementation decision is left open.

## Contract impact

The public and internal LLM contracts add checkpoint and structured interpretation fields plus additive action kinds. Existing legacy fields remain readable; they cannot bypass the new guard. Existing session events, receipts and the worker queue are reused.

## Verification

The evidence bundle maps C-176 and R-45 to pure Creation transition tests, route/replay integration tests, generated-contract checks and architecture checks. Execution evidence is added only after the checks run.

## Residual risks

The only deliberate degradation is a legacy diagram that must be uploaded again. This is necessary because the platform retained its digest rather than the original image.
