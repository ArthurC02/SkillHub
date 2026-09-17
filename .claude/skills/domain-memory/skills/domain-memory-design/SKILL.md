---
name: domain-memory-design
description: Turn a domain-significant requirement into a reviewable DDD boundary and Change Package using the Domain Registry.
---

# Design a Domain Change

Use the read capability first. Use the seven-step workflow for a changed owner, invariant, boundary, event, contract, consistency rule, regulated rule, or irreversible behavior.

1. Start with business behavior and observable acceptance criteria.
2. Identify the owning Context and existing collaboration.
3. Choose the narrowest collaboration surface that preserves consistency, authorization, idempotency, failure, and observability behavior.
4. Create and validate the Requirement, Proposal, Test Obligations, Evidence Bundle, and Draft PR.

Do not turn an absent Registry record into an invented fact. A proposal records uncertainty and requires the configured review before implementation may rely on it. Read [the seven-step workflow](../../references/seven-step-workflow.md).
