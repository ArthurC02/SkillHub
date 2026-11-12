# devctl

`devctl` is Skill Hub's cross-platform developer automation helper. It uses only
the Go standard library so the diagnostic tool does not need its own dependency
bootstrap.

修改本工具後，在 `tools/devctl` 執行 `go test -count=1 ./...`、`go vet ./...`、`golangci-lint run` 與 `golangci-lint fmt --diff`；CI 的工具模組使用相同的驗證入口。

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

`go -C tools/devctl run . gateway-search-proof` 以 Compose 釘住的映像、獨立內部網路與 Go 搜尋／模型替身，驗證真正的簽發器、逐層搜尋授權、預算、撤銷、原生搜尋攔截與費用紀錄，不呼叫付費供應商。YAML 解析沿用閘道映像內建工具，不增加 Go 依賴或主機 Python 需求。暫存執行檔、容器與匿名磁碟由這次執行清理，log 與來源摘要留在 scratchpad；CI 保存 7 天。完整邊界與未完成範圍見[執行時搜尋](../../docs/development/runtime-web-search.md)。

The SQL scope's sources and its committed output are listed once, in the
generation-ownership table in `docs/development/automation.md` ("生成來源與所有權").
A third copy of that table is a third thing to keep in step. Resolve conflicts in
the source, then regenerate—never merge generated Go by hand.

The OpenAPI scope is documented in `tools/codegen/README.md`. It runs the
digest-pinned TypeScript generator and the lockfile-built Python generator,
then replaces only each package's `src/generated` subtree.
