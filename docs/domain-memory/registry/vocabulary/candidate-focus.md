---
type: Term
status: draft
sources:
  - resource: "contracts/openapi/llm-internal.yaml#L2160-2166"
    excerpt_sha256: "sha256:82fc08685aeaf19772f3e70434a595f6714c1b23db5a23aa6e29e6ca221dc854"
  - resource: "apps/llm/src/skillhub_llm/evaluate.py#L531-547"
    excerpt_sha256: "sha256:f8731f7f9c7d118dcc7a63afa4087d250e887e7ab04a768fd3180a82087a13c5"
---
Where one candidate of a batch improvement round is told to look first for the fix: instructions (SKILL.md, its description and when it is used), scripts_and_tools (the bundled scripts and how the instructions call tools and scripts), references_and_examples (the references, examples and data files the instructions point to) or free (anywhere, preferring a change of a different kind). A round of one candidate is asked with no focus.

# Relations
- Contexts: [eval](/contexts/eval)
