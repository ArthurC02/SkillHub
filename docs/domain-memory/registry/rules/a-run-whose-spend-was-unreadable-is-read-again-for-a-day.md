---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/execution/supervisor.go#L89-107"
    excerpt_sha256: "sha256:aadd6c04e5a0f6f30b7cd61fd1b9d30db8ecb7569ec2bf99f39291608d2d8dd4"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/run_credit_integration_test.go#L210-267"
    excerpt_sha256: "sha256:1856434a054972081409699eff0458fc8e20bfe126364c234fd2f736916b979a"
---
A cleaned Run whose cost was not settled at its real spend is queued for another cleanup by supervision, at most once every 5 minutes, until a read settles it or a day has passed since it finished; a settled Run is never queued for spend again.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A Run whose spend was unreadable at cleanup and finished 23 h 59 min ago is queued; one that finished 24 h 1 min ago is not; once the spend reads $0.0382 it is charged 50 credits and no longer queued.
