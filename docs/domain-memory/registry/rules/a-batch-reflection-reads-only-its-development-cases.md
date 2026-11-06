---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/improvement/reflection/reflection.go#L147-181"
    excerpt_sha256: "sha256:9b652a61f6fe2f34584cd34c377eb6ac326e046d3570c2a04597bf0591090c77"
  - resource: "apps/platform/internal/trial/improvement/reflection/reflection.go#L213-216"
    excerpt_sha256: "sha256:ee3cd79f8c21afac92618f8e8aa392d7538e952f4e1612fd518e8b8121cead39"
---
An improvement round over an evaluation batch reads only the failing runs of the test cases the creator chose for development and holds every other case out for checking the result: it needs at least one development case and at least one held-out case, each named once and from this batch, refuses while the batch is launching or while any development trial is not settled, and reads at most eight failing runs, taking every case's first trial before any second and saying how many it left unread.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A batch of three cases with all three chosen for development is refused, and so is a round naming no case or one case twice.
- A development case with a trial still running is refused until it settles, while a held-out case with no runs yet does not hold the round back.
- Nine failing development runs are read as eight, and the round records one run not read.
