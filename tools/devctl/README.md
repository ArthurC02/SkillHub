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

Taskfile wraps the same commands for normal use. The direct Go form is the
fallback on a new machine where Task is not installed yet. Code generation is
added to this command in the next automation phase; do not create a parallel
shell implementation.
