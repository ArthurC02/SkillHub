---
type: Bounded Context
title: Credit Ledger
status: stable
subdomain: supporting
implementation_path: creator/credit
verified:
  - by: "human:ArthurC"
    at: "2026-09-22T10:27:24.111820Z"
sources:
  - resource: "apps/platform/internal/creator/credit/doc.go#L1-3"
    excerpt_sha256: "sha256:8db4bee7c617a7481427d85cb6a175ea5f9769f46221a942cca0b20f666e65ac"
---
Owns the platform's unit of account: a user's credit balance, the ledgers that explain it, and the cost statistics that decide whether a paid call may start.

# Requirement prefixes
- CRED
