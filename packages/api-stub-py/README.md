# skillhub-api-stub

Pydantic v2 models generated from `contracts/openapi/llm-internal.yaml`.

- `src/skillhub_api_stub/generated/**` is generated; **do not edit it**.
- Package metadata and `skillhub_api_stub/__init__.py` are hand-written stable
  boundaries.
- Change the OpenAPI source, run `task gen:openapi`, then run
  `task gen:check` before committing.

The generated models describe transport data only. Policy, authorization,
retry, state transitions and queue ownership remain in Go under ADR-016.
