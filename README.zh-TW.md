<h1 align="center">Skill Hub</h1>

<p align="center">
  以證據、來源溯及與受控執行為核心，探索、創建、試跑與散布 Agent Skill 的開放平台。
</p>

<p align="center">
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
  <a href="https://github.com/ArthurC02/SkillHub/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/ArthurC02/SkillHub/actions/workflows/ci.yml/badge.svg"></a>
  <a href="README.md">English</a>
</p>

> [!IMPORTANT]
> Skill Hub 仍是 pre-release 軟體。repo 內已有可運作的產品路徑，但公開曝光、付費模型使用與正式部署仍受明確設定與驗證約束。把某項能力當作已發布前，請先讀[目前計畫與里程碑狀態](docs/plans/01-goals-and-plan.md)。

## 為什麼需要 Skill Hub

Agent Skill 是否值得使用，不只取決於能不能跑，還包括來源在哪裡、能存取什麼、是否符合自己的任務、花了多少成本，以及成果能不能帶走。Skill Hub 把這些問題做成產品的一部分，而不是留給使用者靠習慣判斷。

- **探索**：以關鍵字與語意搜尋尋找 Skill，同時看見來源、授權、依賴、權限與相容性資訊。
- **創建與改善**：從任務描述或引導式會話創作 Skill，並保留可稽核的版本歷史。
- **試跑**：以明確的 Prompt、測試資料、驗收條件、成本邊界與 Trace 執行凍結的 Skill Version。
- **評估**：將結果判為符合、部分符合、未符合或無法判斷；核可的改善會產生新的不可變版本。
- **打包與散布**：匯出保留來源、授權與選定測試材料的可攜 Skill 套件。

產品刻意不做全域品質排行榜：證據只有放回產生它的任務與驗收條件才有意義。

## 架構總覽

```text
瀏覽器
  │
  ▼
React Web ───────────────► Go 控制平面 API ───► PostgreSQL + S3 相容物件儲存
                                  │                         │
                                  │                         └── transactional outbox
                                  ▼
                           Go Worker（唯一佇列消費者）
                              │                    │
                 internal HTTP│                    │internal HTTP
                              ▼                    ▼
                     Python LLM 能力服務       獨立節點上的 sandboxd
                              │                    │
                              ▼                    ▼
                        LiteLLM 閘道           gVisor Runtime Image
                              │
                              ▼
                          模型供應商
```

Go 控制平面擁有授權、Workspace Scope、領域規則、Run 狀態與全部核心資料。Python LLM 與 Sandbox 是能力提供者：收結構化請求、回結構化結果，但不能直接存取核心資料庫。每次模型呼叫都經 LiteLLM；供應商憑證只留在 Gateway。Run 使用短效 Virtual Key，領域狀態變更與對外事件則在同一交易中寫入。

Platform 程式依 creator、product、skill、trial 的 Bounded Context 組織。外部系統透過 Port 與 Adapter 接入；套件、query 與跨 context 依賴都有機器檢查。詳見[架構身份](apps/platform/architecture-identity.yaml)、[Bounded Context 模型](docs/adr/README.md#platform-bounded-context-與-context-map)與[架構決策](docs/adr/README.md)。

## 安全模型

- 不受信任的 Skill、Script 與上傳資料不會在 Web 或 API 行程執行。
- 執行平面不能連到核心資料庫。
- 使用者資料的 Workspace Scope 來自登入 Session，不信任用戶端傳入的 workspace id。
- Skill Version、Test Case 快照與歷史 Run 都不可變；採用改善＝建立新版本。
- Secret 不得進入套件、Log、Trace 或分析資料；Trace 入庫前會遮罩。
- 正式 Sandbox 節點採 gVisor 與 default-deny egress，節點以換新取代原機修理。

淨測試模式刻意**不是**安全邊界。它用行程內替身讓產品可在沒有 Docker、金鑰與網路的機器展示；不可拿它執行不受信任的 Skill 或真實資料。

## 快速開始：淨測試模式

最快看到產品運作的方式是零成本展示模式。它需要 Go 與 Node.js，不需要 Docker 或模型金鑰。

```bash
task doctor
task bootstrap
npm ci --prefix tools/pglite
npm --prefix apps/web run build
task clean-mode
```

啟動器會印出本機網址；若缺少前提，也會指出原因。沒有 [Task](https://taskfile.dev/) 時，可改用 `go -C tools/devctl run . doctor`、`go -C tools/devctl run . bootstrap` 與 `node tools/cleanmode/start.mjs --seed`。

淨測試模式以嵌入式資料庫、記憶體物件儲存與本機程序 Driver 取代正式元件。它能證明瀏覽器到 API 的產品旅程，不能證明正式隔離、預簽物件 URL、併發、物件儲存或付費模型能力。

## 本機完整開發

先執行可攜診斷。工具版本以 `go.mod`、`.node-version`、`apps/llm/.python-version` 與 [`tools/toolchain.yaml`](tools/toolchain.yaml) 為準，不由本 README 複寫。

```bash
task doctor
task env:init
task bootstrap
task gen:check
task dev
```

`task dev` 啟動不花模型費用的本機 PostgreSQL 與 SeaweedFS。接著在不同終端啟動產品程序：

| 元件 | 指令 | 責任 |
| --- | --- | --- |
| API | `go -C apps/platform run ./cmd/api` | HTTP、身分、授權與領域命令 |
| Worker | `go -C apps/platform run ./cmd/worker` | Run 派送、清理、outbox 與週期工作 |
| LLM 能力服務 | `cd apps/llm && uv run uvicorn skillhub_llm.app:app` | 結構化、模型驅動的能力 |
| Sandbox Provider | `go -C apps/sandbox run ./cmd/sandboxd` | 本機執行提供者邊界 |
| Web | `npm --prefix apps/web run dev` | React 開發介面 |

本機 SPA 需要讓 API 明確設定 `DEV_CORS_ORIGIN=http://localhost:5173`。若要跑真實 Run，API 與 Worker 必須共享同一組資料庫、物件儲存、Sandbox、模型閘道與 Trace 設定；完整依賴與驗證順序見[Provision 手冊](docs/runbooks/provisioning.md)。

### 可選模型能力與成本

模型能力預設關閉：

```bash
task dev:model
task dev:llm
```

前者在檢查必要 Secret 後啟動 LiteLLM；後者替 `apps/llm` 簽發有預算上限的 Virtual Key，而不是把 Gateway master key 交給它。只要請求到模型供應商就可能付費，因此付費 live test 一律 opt-in，不會出現在預設測試命令中。

## 測試與驗證

```bash
task gen:check     # generated contract 與 SQL output 是否仍對齊來源
task test          # 一般測試；付費 E2E 仍是 opt-in
task ci            # 可重現、無 Secret 的本機 CI 流程
task preflight     # 檢查未推送 commit 的 CI 相關規則
```

本機綠燈只證明目前機器的結果，不等於託管 CI 或正式環境已成立。[CI 診斷手冊](docs/runbooks/ci-red.md)說明如何分辨跳過、看不懂與真正失敗的 workflow。修正行為時，測試必須先證明未修正版本會失敗，才能宣稱修好。

## 專案結構

| 路徑 | 用途 |
| --- | --- |
| `apps/web` | React／TypeScript 使用者介面 |
| `apps/platform` | Go 控制平面、Worker 與 Bounded Context |
| `apps/llm` | Python FastAPI 能力提供者 |
| `apps/sandbox` | Go Sandbox Provider |
| `packages` | 可重用 library 與生成的 API client |
| `contracts` | 跨程序 OpenAPI、事件與套件契約的唯一來源 |
| `db` | Migration、query、SQL ownership 與 sqlc 設定 |
| `infra` | Compose、部署、Runtime Image、egress 與可觀測性 |
| `tools` | 開發、CI、資料維護與維運指令 |
| `docs` | 產品計畫、架構決策、設計規範與 Runbook |

## 文件入口

- [產品計畫與目前里程碑狀態](docs/plans/01-goals-and-plan.md)
- [規格與驗收準則](docs/plans/02-specifications-and-acceptance-criteria.md)
- [架構決策](docs/adr/README.md)
- [開發自動化](docs/development/automation.md)
- [遠端開發（Codespaces + Dev Containers）](docs/development/REMOTE_DEVELOPMENT.md)
- [Platform DDD 實務](docs/development/platform-ddd-practices.md)
- [Provision 與維運 Runbook](docs/runbooks/README.md)
- [`AGENTS.md`](AGENTS.md)：人與 Coding Agent 都要遵守的 repo 規則

## 貢獻

歡迎貢獻。請先讀 [CONTRIBUTING.md](CONTRIBUTING.md)，generated 檔一律由來源生成，跨程序介面先改 `contracts/`，並依改動範圍執行對應檢查。不得提交 `.env`、憑證、付費測試輸出或任何 Secret。

資安問題不要開 public issue，請走 [SECURITY.md](SECURITY.md) 的私下回報程序。

## 授權

[MIT](LICENSE)
