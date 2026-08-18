# Contract code generation

`task gen:openapi` is the only supported entry point. `tools/devctl` runs both
generators under the shared-worktree lock, validates deterministic content, and
atomically replaces only generated subdirectories.

| Source | Generator | Generated target |
| --- | --- | --- |
| `contracts/openapi/public.yaml` | OpenAPI Generator 7.19.0 `typescript-fetch`, digest-pinned container | `packages/api-client-ts/src/generated/` |
| `contracts/openapi/llm-internal.yaml` | datamodel-code-generator 0.35.0/Pydantic v2, fully locked image in `python/` | `packages/api-stub-py/src/skillhub_api_stub/generated/` |

The Python image uses a digest-pinned Python base, copies uv from its own pinned
image, and installs the complete `uv.lock`; do not replace it with an unpinned
`uvx` call. Generated output excludes timestamps and is rejected if it embeds a
repository absolute path.

When a contract changes:

1. edit the OpenAPI source;
2. run `task gen:openapi` as the single Writer;
3. review semantic changes in generated output;
4. run `task gen:check` plus the consuming package tests;
5. stage the source, generated output and affected lockfiles explicitly.

Do not use `--skip-validate-spec`, maintain an OpenAPI 3.0 shadow copy, or edit a
generated file to work around a generator failure. Resolve an equivalent 3.1
schema expression or revise ADR-030 before changing the tool boundary.
