# 遠端開發（GitHub Codespaces / VS Code Dev Containers）

本文件說明 SkillHub 的遠端開發標準設定：同一份 `.devcontainer/devcontainer.json` 同時服務 GitHub Codespaces 與本機 VS Code Dev Containers。

## 快速開始

### GitHub Codespaces

1. 進入你的 SkillHub repository 頁面。
2. 點 **Code → Codespaces → Create codespace on `<branch>`**。
3. 等待容器建立，`postCreateCommand` 會自動完成 `.env` 初始化與依賴安裝。
4. 開新終端執行（`task doctor` 是建議檢查，`task clean-mode` 才是啟動模式）：

```bash
task doctor
task clean-mode
```

### VS Code Dev Containers（本機 Docker）

1. 安裝 VS Code 的 Dev Containers 擴充。
2. 開啟 repo 後執行 **Reopen in Container**。
3. 等待容器完成初始化後執行（`task doctor` 是建議檢查，`task clean-mode` 才是啟動模式）：

```bash
task doctor
task clean-mode
```

## Codespaces 與本機 Dev Container 對比

| 面向 | GitHub Codespaces | 本機 Dev Container |
| --- | --- | --- |
| 啟動位置 | GitHub 雲端 VM | 本機 Docker Engine |
| 建議用途 | 零本機安裝、快速上手、跨裝置開發 | 低延遲、完整本機資源控制 |
| 計費 | 依 GitHub 配額與方案 | 無 Codespaces 計費（本機資源成本） |
| 資料持久性 | Codespace 儲存體 | 本機檔案系統與 volume |
| Dev Container 設定來源 | `.devcontainer/devcontainer.json` | `.devcontainer/devcontainer.json` |

## 內建工具與 VS Code 擴充

容器內建：

- Go（API、Worker）
- Node.js / npm（Web）
- Python / uv（LLM）
- Sandbox Provider 執行檔（容器內可直接啟動開發流程）
- Docker CLI + Docker Compose（DinD）
- Task CLI、golangci-lint

自動安裝的 VS Code 擴充：

- `golang.go`
- `ms-python.python`
- `ms-python.vscode-pylance`
- `dbaeumer.vscode-eslint`
- `esbenp.prettier-vscode`
- `charliermarsh.ruff`
- `ms-azuretools.vscode-docker`
- `redhat.vscode-yaml`

預設編輯器行為：

- `formatOnSave` 預設開啟。
- 預設 formatter：JavaScript/TypeScript 使用 Prettier，Go 使用 Go extension，Python 使用 Ruff。
- 預設啟用 `python.testing.pytestEnabled=true`，在遠端容器裡以 pytest 作為 Python 測試 runner。

## 偵錯與埠轉發

預設會自動轉發：

- `5173`：Web dev server
- `8080`：Platform API
- `4000`：LiteLLM gateway
- `5432`：PostgreSQL
- `8333`：SeaweedFS S3 API

遠端偵錯保留埠：

- `2345`：Go (Delve)
- `5678`：Python (debugpy)
- `9229`：Node.js inspector

## 環境變數與模式

- `postCreateCommand` 會執行 `.devcontainer/post-create.sh`，腳本會做工具檢查，並呼叫 `go -C tools/devctl run . env-init`。
- `env-init` 會在 `.env` 不存在時由 `.env.example` 建立；若 `.env` 已存在則保持原值不覆寫。
- 若你希望先準備遠端最小模板，可先將 `.devcontainer/.env.remote.example` 複製成 `.env`，再依需求補齊值；該模板採 `localhost` 端點，對應 `task dev` 在 DinD 內綁定的本機埠。
- `post-create.sh` 在完整初始化時要求以下 key 為非空：`DATABASE_URL`、`OBJSTORE_ENDPOINT`、`OBJSTORE_ACCESS_KEY`、`OBJSTORE_SECRET_KEY`、`LITELLM_BASE_URL`。
- 因為 devcontainer 設定 `waitFor=postCreateCommand`，若上述 key 缺值，容器會在初始化階段就失敗；建議先準備好 `.env` 再建立 Codespace/Container。

常見模式：

1. **乾淨測試模式（無外部依賴）**
   - `task clean-mode`
2. **完整開發流程（基礎依賴 + 服務程序）**
   - （容器初始化已自動執行 `env-init` 與 `bootstrap`）
   - `task dev`（只會啟動 PostgreSQL 與 SeaweedFS）
   - 另開終端啟動 API / Worker / Sandbox / LLM / Web
   - 若要手動重建依賴，再執行 `task env:init`、`task bootstrap`

## Codespaces 最佳化重點

- 設定 `hostRequirements`（4 CPU / 8 GB RAM / 32 GB 儲存）作為建議最小規格。
- 使用 named volumes 持久化 Go module、npm、uv 與 Docker layer 快取。
- `post-create.sh` 用於首次建立容器的完整初始化；`updateContentCommand` 以 `SKILLHUB_SKIP_BOOTSTRAP=1` 模式只做輕量檢查與 `.env` 初始化。
- `updateContentCommand` 也會執行同一支腳本，讓 Codespaces prebuild/更新內容時沿用同一初始化流程。

## 疑難排解

### 建立後工具缺失或初始化失敗

在容器內重跑：

```bash
bash .devcontainer/post-create.sh
```

### Docker-in-Docker 無法使用

確認 `postStartCommand` 已完成，並檢查：

```bash
docker info
```

若失敗，查看 `/tmp/dockerd.log`。

### 要切換到完整模型能力

先在 `.env` 填入 `OPENAI_API_KEY`、`LITELLM_MASTER_KEY` 等必要值，再執行：

```bash
task dev:model
task dev:llm
```
