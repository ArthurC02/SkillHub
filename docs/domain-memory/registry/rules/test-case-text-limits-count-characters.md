---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/design/testlab.go#L531-541"
    excerpt_sha256: "sha256:ca340efe5e63b62f8f2af103ecbee81a1471dd450f47aaa5239a53f8225b7045"
---
A Test Case's name and prompt must be non-blank and within their limits, counted in characters so Han text is not penalised for its byte length.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- A prompt of MaxPromptChars+1 Han characters is refused; exactly MaxPromptChars is accepted.
