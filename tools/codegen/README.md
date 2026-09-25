# Contract code generation

`task gen:openapi` is the only supported entry point. `tools/devctl` runs both
generators under the shared-worktree lock, validates deterministic content, and
atomically replaces only generated subdirectories.

| Source | Generator | Generated target |
| --- | --- | --- |
| `contracts/openapi/public.yaml` | OpenAPI Generator 7.25.0 `typescript-fetch`, digest-pinned container | `packages/api-client-ts/src/generated/` |
| `contracts/openapi/llm-internal.yaml` | datamodel-code-generator 0.76.2/Pydantic v2, fully locked image in `python/` | `packages/api-stub-py/src/skillhub_api_stub/generated/` |
| `contracts/openapi/public.yaml` | ogen 1.24.0 models-only, fully locked image in `go/` | `apps/platform/internal/entrypoint/api/gen/` |

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
schema expression or revise the [開發自動化與依賴治理](../../docs/adr/README.md#開發自動化與依賴治理) decision before changing the tool boundary.

## The Go side generates models, not a server

`go/ogen.yaml` disables `paths/client`, `paths/server`, `webhooks/client` and
`webhooks/server`, so the generated package carries request and response types
and nothing that serves a route. Every route is mounted by hand in
`internal/entrypoint/api/apiserver/router.go` and carries its own
`RequireSession` / `RequireOperator` / `OptionalSession` wrapper, which keeps the
whole AuthN/AuthZ matrix visible in one reviewed file.

`TestTheGeneratedGoPackageCarriesModelsAndNoServer` fails if the generated
directory grows a `NewServer`, an `UnimplementedHandler` or a `ServeHTTP`: a
route the generated code appears to serve is a route nobody guards. It also
fails if the directory carries no models, so it cannot pass on an empty
directory.

`GET /healthz` is a hand-written handler that returns the generated `Health`
model, so the one response the contract fully specifies still comes from the
contract's own type. The accepted cost of generating these models is the
ogen/jx/OpenTelemetry runtime dependencies pinned in `apps/platform/go.mod`
and `go.sum`.
