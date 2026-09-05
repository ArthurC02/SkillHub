# apps/platform — 控制平面服務卡

## 必讀來源

- [根指示](../../AGENTS.md) 與 [開發自動化](../../docs/development/automation.md)
- [平台內部指標](internal/AGENTS.md)；若要動 `internal/`，再讀目標套件的 `doc.go`
- [平台內部產品價值流](internal/README.md)、[public API 契約](../../contracts/openapi/public.yaml)
- [根 Taskfile](../../Taskfile.yml) 的 `test:platform`、`lint:platform`、`build:platform`

## 區域權限

這是 Go 控制平面，包含 API、Worker、maintenance 與 reindex 進程；服務層入口由 `internal/entrypoint/api/apiserver.NewApp` 等 composition root 注入。Bounded Context、query ownership 與跨 context 協作只依 `internal/AGENTS.md`、各 `doc.go`、ADR-032～035；本卡不複製 DDD 細節。

## 安全限制

- 平台是唯一控制平面；Run 狀態、授權、政策、重試與 outbox 規則留在 Go。
- 不在 Web/API 內執行不受信任 Skill、Script 或資料；執行平面只能透過 provider 契約互動。
- LLM 呼叫只能經 LiteLLM；Sandbox／LLM 的服務邊界以各自卡片與 OpenAPI 契約為準。
- 變更 `contracts/`、`db/`、generated 目錄、lockfile 或 Taskfile 須由主 Agent 序列化處理。

## 驗證入口

以根 Taskfile 為準：`task test:platform`、`task lint:platform`、`task build:platform`；跨區或文件機器檢查由主 Agent 執行 `task automation:check` 與 `task gen:check`。不要用本卡取代 `internal/AGENTS.md` 的 package 細節。
