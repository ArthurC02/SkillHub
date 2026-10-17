---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:47:28.319050Z"
sources:
  - resource: "apps/platform/internal/trial/design/snapshot.go#L298-300"
    excerpt_sha256: "sha256:9184525d33d316f636139c556128d84a195b3a88a5b01fd0ffc482d021b9c722"
  - resource: "apps/platform/internal/trial/design/check_test.go#L39-47"
    excerpt_sha256: "sha256:d67c82b25b4624ba0ed8dcefd49a355fb7f8d332118115cebc7010a0360bbec7"
---
A continued snapshot's prompt may not exceed MaxPromptChars characters, counted as characters, not bytes.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- A prompt one character over the limit is refused.
