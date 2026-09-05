# apps/sandbox — Sandbox Provider 服務卡

## 必讀來源

- [根指示](../../AGENTS.md) 與 [開發自動化](../../docs/development/automation.md)
- [服務 README](README.md)、[sandboxd 入口](cmd/sandboxd/main.go)
- [Sandbox Provider 契約](../../contracts/openapi/sandbox-provider.yaml)
- [根 Taskfile](../../Taskfile.yml) 的 `test:sandbox`；服務 README 的 Docker build／lint 指令

## 區域權限

這是獨立 Go module 的執行平面 Provider；由 Go 控制平面以契約呼叫，負責 capability、attempt lifecycle、trace／artifact 搬運與隔離執行。它不連 PostgreSQL、不匯入 `apps/platform/internal`、不主動回呼平台，也不在服務程序解壓或執行 Skill 內容。

## 安全限制

- 每個 attempt 必須遵守非 root、唯讀 rootfs、drop capabilities、不得暴露宿主敏感路徑或 Docker 管理 socket 給工作負載、資源上限與網路 default-deny 基線；必要的工作目錄交接依服務實作處理，Secrets 不進 log。
- 生產執行節點使用 Linux gVisor `runsc`、digest-pinned runtime image 與部署期出口控制；本機 runc／Docker 綠燈不等於部署安全性。
- SEC-009 逃逸與 SBX-010 隔離驗收未通過前，不得開放外部使用者提交 Skill 執行；權威條款見 [ADR-015](../../docs/adr/ADR-015-sandbox-isolation-technology.md)、[ADR-022](../../docs/adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 與 [SEC-009 工具](../../tools/sec009/README.md)。
- 契約、runtime image 與 generated 輸出屬序列化區域，不在本卡自行改動或繞過驗證。

## 驗證入口

根入口先用 `task test:sandbox`；服務 README 補充 Docker 版 `go build ./...`、`go vet ./...`、`go test ./...` 與 lint。部署安全須另行完成目標 Linux `runsc`、[ADR-015](../../docs/adr/ADR-015-sandbox-isolation-technology.md)、[ADR-022](../../docs/adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md)、[SEC-009 工具](../../tools/sec009/README.md) 與 SBX-010 驗收，不能以 local test 代替。
