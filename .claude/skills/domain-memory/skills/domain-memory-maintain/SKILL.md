---
name: domain-memory-maintain
description: Keep file-backed Domain Memory current after implementation or source changes by preserving evidence and creating governed candidate updates.
---

# Maintain Domain Memory

Use this after implementation and whenever a selected source moves.

1. Run `verify-sources`, `verify-evidence`, and `verify-audit`.
2. Compare the implemented behavior with the reviewed terms, owner, invariants, boundaries, events, contracts, and capabilities.
3. If the model changed, update the smallest affected asset with `upsert-candidate`, or prepare a Change Package for a material change.
4. Validate the complete Registry and preserve the audit trail.

Never hide a domain change in code-only edits. Candidates prepare the next review and do not authorize implementation. Use only the controlled file-backed commands; do not hand-edit reviewed records or policy. Read [registry maintenance](../../references/registry-maintenance.md).
