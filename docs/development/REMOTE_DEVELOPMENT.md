# 遠端開發（GitHub Codespaces / VS Code Dev Containers）

本文件說明 SkillHub 的遠端開發標準設定：同一份 `.devcontainer/devcontainer.json` 同時服務 GitHub Codespaces 與本機 VS Code Dev Containers。

## 快速開始

### GitHub Codespaces

1. 進入 `ArthurC02/SkillHub` repo。
2. 點 **Code → Codespaces → Create codespace on `<branch>`**。
3. 等待容器建立，`postCreateCommand` 會自動完成 `.env` 初始化與依賴安裝。
4. 開新終端執行：

```bash
task doctor
task clean-mode
```

### VS Code Dev Containers（本機 Docker）

1. 安裝 VS Code 的 Dev Containers 擴充。
2. 開啟 repo 後執行 **Reopen in Container**。
3. 等待容器完成初始化後執行：

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

- Go（API、Worker、Sandbox）
- Node.js / npm（Web）
- Python / uv（LLM）
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

## 偵錯與埠轉發

預設會轉發常用埠：

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

- `postCreateCommand` 會執行 `.devcontainer/post-create.sh`，腳本會做工具檢查、`.env` 初始化（僅在不存在時建立）與必要 key 驗證。
- 若你希望先準備遠端最小模板，可參考 `.devcontainer/.env.remote.example`。

常見模式：

1. **乾淨測試模式（無外部依賴）**
   - `task clean-mode`
2. **完整堆疊模式（PostgreSQL + SeaweedFS + 各服務）**
   - `task env:init`
   - `task bootstrap`
   - `task dev`
   - 另開終端啟動 API / Worker / Sandbox / LLM / Web

## Codespaces 最佳化重點

- 設定 `hostRequirements`（4 CPU / 8 GB RAM / 32 GB 儲存）作為建議最小規格。
- 使用 named volumes 持久化 Go module、npm、uv 與 Docker layer 快取。
- `post-create.sh` 用 lockfile 雜湊判斷是否需要重跑 bootstrap，減少重建時間。
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
