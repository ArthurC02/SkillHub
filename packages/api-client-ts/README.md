# @skillhub/api-client-ts

Model types and enum constants generated from `contracts/openapi/public.yaml`
by `tools/codegen/ts`. There is no generated HTTP client: `apps/web` talks to
the API through its own `core/api/client.ts` and imports types from here.

- `src/generated/**` is generated; **do not edit it**.
- `src/index.ts`, package metadata and the lockfile are hand-written boundaries.
- Change the OpenAPI source, run `task gen:openapi`, then run
  `task gen:check` before committing.

The Web API wrappers remain endpoint-specific adapters. They map transport DTOs
to UI view models one endpoint at a time; generated types do not become product
policy, and the app does not keep an unused catch-all client factory.
