# @skillhub/api-client-ts

Typed browser client generated from `contracts/openapi/public.yaml` with the
pinned OpenAPI Generator `typescript-fetch` target.

- `src/generated/**` is generated; **do not edit it**.
- `src/index.ts`, package metadata and the lockfile are hand-written boundaries.
- Change the OpenAPI source, run `task gen:openapi`, then run
  `task gen:check` before committing.

The existing Web API wrappers remain in place during incremental adoption. The
first integration only creates a cookie-enabled `generatedApi` factory; it does
not replace an endpoint. Wrappers
can map generated transport DTOs to UI-specific view models one endpoint at a
time; generated types do not become product policy.
