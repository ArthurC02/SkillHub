---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T15:07:58.144675Z"
sources:
  - resource: "apps/platform/internal/skill/publishing/service.go#L307-323"
    excerpt_sha256: "sha256:8e20a66851358fcdbbbef73bb27fd504a71426cea2a292a1d4bce0db3fe146b6"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/publishing_release_guard_integration_test.go#L24-42"
    excerpt_sha256: "sha256:1bbbbd7537384980cd26e261152236ba47ff7e0a95423d4ae9b5acf786caa793"
---
Publishing a Skill with a version of another Skill is answered not found, so one Skill's gates never pass another Skill's files.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- Publishing Skill A with Skill B's version answers 404.
