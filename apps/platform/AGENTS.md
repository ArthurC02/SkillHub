# apps/platform — 控制平面服務卡

## 必讀來源

- [根指示](../../AGENTS.md) 與 [開發自動化](../../docs/development/automation.md)
- [平台內部指標](internal/AGENTS.md)；若要動 `internal/`，再讀目標套件的 `doc.go`
- [Platform DDD 收斂指南](../../docs/development/platform-ddd-convergence.md)；首次接手既有 Aggregate、Port 或 Store 重構時先讀相關段落
- [平台內部產品價值流](internal/README.md)、[public API 契約](../../contracts/openapi/public.yaml)
- [根 Taskfile](../../Taskfile.yml) 的 `test:platform`、`lint:platform`、`build:platform`

## 區域權限

這是 Go 控制平面，包含 API、Worker、maintenance 與 reindex 進程；服務層入口由 `internal/entrypoint/api/apiserver.NewApp` 等 composition root 注入。Bounded Context、query ownership 與跨 context 協作只依 `internal/AGENTS.md`、各 `doc.go`、[Domain Memory](../../docs/domain-memory/) 的已審查 Context 與 dependency policy 與 [ADR 索引](../../docs/adr/README.md)；本卡不複製 DDD 細節。

## 平台紅線

1. **Go 側只生成 models，沒有 generated router 可以 mount**：contract 的 Go 產出是型別，server 與 client 都不生成。每一條 route 在 `router.go`（登入與 `/me` 在 `creator/workspace` 的 `Mount`）逐條掛上並逐條套 `RequireSession`／`RequireOperator`／`OptionalSession`，每加一條要加 route 測試。重新打開 server 生成會被 `TestTheGeneratedGoPackageCarriesModelsAndNoServer` 擋下。
2. **Bounded Context 治理**：每個套件屬於且僅屬於一個 context，新套件先登記再建目錄（Bounded Context 進 [Registry](../../docs/domain-memory/registry/contexts/index.md)，技術套件進 `architecture-identity.yaml`；兩者都有機器對帳）。跨 context 的新 import 先在 Registry 立下 dependency policy，**同一個 commit** 改 depguard。領域 Service 只由 `entrypoint/api/apiserver.NewApp` 注入，禁止方法內現場建構。日常判斷見 [platform-ddd-practices.md](../../docs/development/platform-ddd-practices.md)。
3. **Query ownership**：owner 宣告在 `db/query-owners.yaml`，跨 context 呼叫會 FAIL；新增或刪除 query 同一批改該檔；`allow:`／`read_allow:` 是存量漂移清單，**不是擴充點**。

## 安全限制

- 平台是唯一控制平面；Run 狀態、授權、政策、重試與 outbox 規則留在 Go。
- 不在 Web/API 內執行不受信任 Skill、Script 或資料；執行平面只能透過 provider 契約互動。
- LLM 呼叫只能經 LiteLLM；Sandbox／LLM 的服務邊界以各自卡片與 OpenAPI 契約為準。
- 變更 `contracts/`、`db/`、generated 目錄、lockfile 或 Taskfile 須由主 Agent 序列化處理。

## 驗證入口

以根 Taskfile 為準：`task test:platform`、`task lint:platform`、`task build:platform`；跨區或文件機器檢查由主 Agent 執行 `task automation:check` 與 `task gen:check`。不要用本卡取代 `internal/AGENTS.md` 的 package 細節。
