---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-08T06:58:44.021319Z"
sources:
  - resource: "apps/platform/internal/trial/design/testlab.go#L531-541"
    excerpt_sha256: "sha256:46d06d95bb74328f72274a752c28fe0c91211cac0724c64e6282c8dbf75f704a"
  - resource: "apps/platform/internal/trial/design/char_limits_test.go#L9-37"
    excerpt_sha256: "sha256:0b9e482eae35cb808d2c721fc1ff6d5998507119ad70512da86b3a3d4fd01051"
---
A Test Case's name and prompt must be non-blank and within their limits, counted in characters so Han text is not penalised for its byte length.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- A prompt of MaxPromptChars+1 Han characters is refused; exactly MaxPromptChars is accepted.
