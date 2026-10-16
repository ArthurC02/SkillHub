---
type: Bounded Context
title: Operations
status: stable
subdomain: supporting
implementation_path: product/operations
requirement_prefixes: []
verified:
  - by: "human:ArthurC"
    at: "2026-10-07T06:11:01.778535Z"
sources:
  - resource: "apps/platform/internal/product/operations/doc.go#L1-3"
    excerpt_sha256: "sha256:60b4b87f6611a465e4b6d26509322606eb04f67b9d154407f465c1a15bddc3a5"
---
Owns the platform's own agents: who each one is, whether it may run, the brake that stops them all, and the record of every run.
