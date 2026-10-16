---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-09-22T11:40:29.217542Z"
sources:
  - resource: "docs/adr/ADR-016-platform-bounded-contexts-and-context-map.md#L53-53"
    excerpt_sha256: "sha256:5665a77570ab86754c4b5578e93d83c1657f67ffcc095b8bd8e4f5723b6b0c9b"
---
Interactive Skill creation does not import the Contexts it collaborates with, and none of them imports it. Everything it needs from the reference, admission and trial Contexts arrives as a narrow interface injected by a composition root, so the creation loop can be replaced or removed without any other Context changing.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- Enqueueing a creation job goes through an interface the wiring package supplies, not through the queue Context's own Service.
- A Context that wants to know what a creation session produced reads the Skill Version the session created, not the session.
- Credit and identity are reached the same way every other Context reaches them, because neither is part of the creation loop being kept replaceable.
