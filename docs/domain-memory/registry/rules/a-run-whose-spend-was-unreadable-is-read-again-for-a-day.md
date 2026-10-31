---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-31T17:39:54.456397Z"
sources:
  - resource: "apps/platform/internal/trial/execution/supervisor.go#L89-107"
    excerpt_sha256: "sha256:f6a13bb325fe8454a48ace4121ae9549c114b233b3ce2559741d657e72e668a0"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/run_credit_integration_test.go#L210-267"
    excerpt_sha256: "sha256:1856434a054972081409699eff0458fc8e20bfe126364c234fd2f736916b979a"
---
A cleaned Run whose cost was not settled at its real spend is queued for another cleanup by supervision, at most once every 5 minutes, until a read settles it or a day has passed since it finished; a settled Run is never queued for spend again.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A Run whose spend was unreadable at cleanup and finished 23 h 59 min ago is queued; one that finished 24 h 1 min ago is not; once the spend reads $0.0382 it is charged 50 credits and no longer queued.
