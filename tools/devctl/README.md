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

Taskfile wraps the same commands for normal use. The direct Go form is the
fallback on a new machine where Task is not installed yet. Generator versions
come from `tools/toolchain.yaml`; do not create a parallel shell or CI
implementation.

The SQL scope's only source files are `db/migrations/**`,
`db/queries/**` and `db/sqlc.yaml`; its committed output is
`apps/platform/internal/foundation/persistence/db/gen/**`. Resolve conflicts in the
source, then regenerate—never merge generated Go by hand.

The OpenAPI scope is documented in `tools/codegen/README.md`. It runs the
digest-pinned TypeScript generator and the lockfile-built Python generator,
then replaces only each package's `src/generated` subtree.
