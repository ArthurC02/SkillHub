---
type: Bounded Context
title: "Identity & Workspace"
status: stable
subdomain: core
implementation_path: creator/workspace
verified:
  - by: "human:ArthurC"
    at: "2026-09-22T10:27:24.111820Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/doc.go#L1-2"
    excerpt_sha256: "sha256:306577f02884d5012e8d340a6b1b3593b7e118fc573bb71c7d47a50f331fa176"
---
Owns creator accounts and workspaces: login, sessions, and resolving the authenticated user and their personal workspace.

# Requirement prefixes
- WS
- SEC
