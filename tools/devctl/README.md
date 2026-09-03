# devctl

`devctl` is Skill Hub's cross-platform developer automation helper. It uses only
the Go standard library so the diagnostic tool does not need its own dependency
bootstrap.

From the repository root:

- `go -C tools/devctl run . doctor` checks required native tools without printing
  environment values.
- `go -C tools/devctl run . env-init` creates `.env` from `.env.example` and
  never overwrites an existing file.
- `go -C tools/devctl run . bootstrap` downloads Go, npm and uv dependencies.
- `go -C tools/devctl run . profile-check model` fails before model
  infrastructure starts when required variables are absent, without printing
  their values.
- `go -C tools/devctl run . gen` regenerates committed outputs under an
  advisory single-writer lock. It generates into same-filesystem scratch space
  and replaces a target only after the complete tree is ready.
- `go -C tools/devctl run . gen --check` compares temporary output with the
  committed tree and never modifies tracked files. It writes only transient
  files under the gitignored `.devctl/` directory. CI uses this exact path.
- `go -C tools/devctl run . test-report <dir> [go test args]` runs `go test` in
  a module and adds what the plain summary omits: how many tests skipped and
  the reason each one gave, grouped. It echoes output only for failures, and
  exits with go test's own code. Reporting only — the switch that turns a
  database skip into a failure is `SKILLHUB_REQUIRE_DB=1`, read by each
  affected `TestMain` and enforced across packages by `automation-check`
  (02:PORT-004).

Taskfile wraps the same commands for normal use. The direct Go form is the
fallback on a new machine where Task is not installed yet. Generator versions
come from `tools/toolchain.yaml`; do not create a parallel shell or CI
implementation.

The SQL scope's sources and its committed output are listed once, in the
generation-ownership table in `docs/development/automation.md` ("生成來源與所有權").
A third copy of that table is a third thing to keep in step. Resolve conflicts in
the source, then regenerate—never merge generated Go by hand.

The OpenAPI scope is documented in `tools/codegen/README.md`. It runs the
digest-pinned TypeScript generator and the lockfile-built Python generator,
then replaces only each package's `src/generated` subtree.
