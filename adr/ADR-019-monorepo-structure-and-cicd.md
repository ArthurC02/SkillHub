# ADR-019：Monorepo 目錄結構與 CI/CD

- 狀態：Proposed
- 日期：2026-08-14
- 決策者：產品負責人、架構規劃

## 背景

[AGENTS.md](../AGENTS.md) 明列「Monorepo 目錄結構與 CI/CD **尚未決策**——不要自行發明結構開始鋪程式碼，先提案」。負責人已指示開工，M1 要落地的工作量（`plans/mvp/03-work-items.md` 第 5～8 節：CORE-001~008、INGEST-001~010、DISC-001~010、WS-001~006）橫跨三種語言與資料庫 Migration，沒有共同結構就無法開始。

決策驅動因素：

1. [ADR-016](./ADR-016-language-and-framework-selection.md) 定了三語言：TS 前端、Go 平台、Python LLM 服務，各有原生工具鏈。
2. 實作鐵律 12：跨語言介面先寫 OpenAPI schema 再實作，**CI 以 codegen 檢查 drift**——這對 repo 拓撲有硬性要求。
3. 實作鐵律 2：執行平面不得直接存取核心資料庫——程式碼佈局應讓這條「預設成立」，而不是靠審查記得。
4. [ADR-010](./ADR-010-mvp-deployment-and-evolution-path.md) 的部署單元是「少量、可拆分」；`plans/mvp/m0/cost-estimation.md` §7.2 定 MVP 走 **E1（docker compose 或 k3s 單機，控制平面全容器化）**，因此 CI 的產出物是容器 Image。
5. 單人／小團隊開發，任何需要維護的建置基礎設施都是直接成本。

本 ADR 只決定 repo 佈局、建置入口與 CI 管線。**部署管線（環境晉升、Migration 與 Rollback）依 ADR-018 定案後另行展開**，本文只寫方向。

## 評估選項

### 一、Repo 拓撲

#### 選項 A：單一 monorepo（採用）

- 優點：OpenAPI spec 與三語言的 codegen 產物在**同一個 commit** 內，drift 檢查退化成「重跑 codegen 後 `git diff` 必須乾淨」，不需要跨 repo 版本協調（鐵律 12 的最低成本實作）；跨語言的原子性變更（改 spec ＋ 改 Go handler ＋ 改前端呼叫）是一個 PR；只有一套 CI 設定要維護。
- 缺點：CI 若不做 path filter，改一行前端也會跑 Go 測試；repo 內語言混雜，工具的預設全域掃描（linter、IDE 索引）需要排除設定。

#### 選項 B：前端 repo ＋ 後端 repo

- 優點：前端可獨立發版與部署，符合多數團隊習慣。
- 缺點：TS client 是 Go spec 的 codegen 產物，跨 repo 後必須靠版本化套件發佈才能同步，等於為了拆分而引入一套套件發佈流程；契約 drift 從「CI 一個 diff 檢查」變成「兩個 repo 的版本相容矩陣」。**直接與鐵律 12 的成本假設衝突。**

#### 選項 C：Go repo ／ Python repo ／ Web repo 三分

- 優點：與 ADR-016 的語言邊界一一對應，各 repo 的工具鏈最乾淨。
- 缺點：B 的缺點乘以二；且 ADR-016 守則 3（Go Worker 以內部 HTTP 呼叫 Python）的介面變更需要三 repo 協調發版。MVP 階段沒有任何獨立發版需求要買這個代價。

**結論**：採 A。B／C 的優點全部指向「獨立發版」，而 ADR-010 明列服務拆分須由實際壓力觸發；拆 repo 是比拆服務更難回頭的動作，現在做等於預付成本。

### 二、建置編排

#### 選項 D：各語言原生工具 ＋ 薄共同入口（採用）

- 工具：`go build` / `go test`、Vite ＋ 套件管理器 script、`uv` ＋ `pytest`。共同入口只做「把常用指令記在一個檔案裡」，不做依賴圖、不做快取、不做增量建置。
- 優點：零學習成本；任何一位開發者用該語言的標準指令都能直接work；CI 的 job 定義就是原生指令。
- 缺點：跨語言的「改了什麼要重建什麼」靠 CI 的 path filter 手寫，規模大時會漏。

#### 選項 E：Bazel

- 優點：真正的跨語言依賴圖與遠端快取，能精確回答「這個 commit 要重跑哪些測試」。
- 缺點：三語言的 rule set 都要維護；Go／Python／Node 的第三方依賴都要進 Bazel 的依賴管理，等於放棄 `go.mod`／`uv.lock` 的原生體驗。**單人團隊的建置時間不是瓶頸，買不到對應的價值。否決。**

#### 選項 F：Nx／Turborepo

- 優點：比 Bazel 輕，工作快取與 task 圖現成。
- 缺點：生態以 JS/TS 為中心，Go 與 Python 只能當「執行外部指令的黑盒」，拿不到細粒度快取——也就是最想要的那一半拿不到。**否決。**

**重評條件（滿足任一則重開建置工具評估）**：CI 單次 PR 的平均等待時間 > 10 分鐘；或 CI 分鐘數成為實質成本；或參與開發者 ≥ 3 人且開始出現「不知道該跑哪些測試」的實際事故。

## 決策

### 1. 目錄結構

> **Amended by [ADR-024](./ADR-024-top-level-repository-layout.md)（2026-08-16）**：頂層改分「跑的」（`apps/`、`services/`、`contracts/`、`db/`、`infra/`、`tools/`）與「讀的」（`docs/`）。下方樹狀圖中的 `spikes/`、`plans/`、`adr/` 三行已純前綴搬移為 `docs/spikes/`、`docs/plans/`、`docs/adr/`（目錄名不變），CI 的 `!spikes/**` 排除項改為 `docs/**`。本節其餘內容不受影響。

單一 monorepo，頂層依「部署產物」而非「技術層」分組：

```text
skillhub/
├─ AGENTS.md / CLAUDE.md / LICENSE / Taskfile.yml
├─ .github/workflows/          # CI 定義（見第 3 節）
├─ apps/
│  └─ web/                     # M1。React + TS + Vite SPA（ADR-016）；DESIGN-*／DISC-*／WS-* 的 UI
├─ services/
│  ├─ platform/                # M1。Go 控制平面：chi/echo + pgx + sqlc + River（ADR-016、014）
│  │  ├─ cmd/api/              #   HTTP API 程序（ADR-010 部署單元 2）
│  │  ├─ cmd/worker/           #   River 佇列消費者（ADR-010 部署單元 3；鐵律 7）
│  │  ├─ internal/<domain>/    #   ADR-002 的九個領域模組各一個 package，跨模組只走公開 API
│  │  └─ internal/platform/    #   共用基礎設施：db、queue、httpx、telemetry（不含業務規則）
│  ├─ llm/                     # M1。Python 內部服務：FastAPI + LangGraph + uv（ADR-016）
│  │  ├─ src/skillhub_llm/     #   索引增強、查詢改寫、LLM Judge、改善建議（DISC-001/002、EVAL-*）
│  │  └─ tests/
│  └─ sandbox/                 # M2 才建立。執行平面 Worker，獨立 Go module（見下方「鐵律 2 的結構性保證」）
├─ contracts/                  # OpenAPI-first 的唯一 spec 來源（鐵律 12）
│  ├─ openapi/public.yaml      #   Web ↔ Go 的對外 API
│  ├─ openapi/llm-internal.yaml#   Go → Python 內部 API（ADR-016 守則 3）
│  └─ events/                  #   領域事件 schema（ADR-008；M2 起用）
├─ packages/                   # codegen 產物，一律入 repo，人工不得編輯
│  ├─ api-client-ts/           #   給 apps/web 的 TS client
│  └─ api-stub-py/             #   給 services/llm 的 FastAPI server stub 與型別
├─ infra/
│  ├─ compose/                 # E1 部署形態：本機開發與單機部署的 compose 檔（cost-estimation §7.2）
│  └─ images/                  # 各服務的 Dockerfile 與 Runtime Image 定義（SBX-002 於 M2 加入）
├─ db/                         # PostgreSQL 唯一真相（ADR-014）
│  ├─ migrations/              #   版本化 Migration，只增不改（CORE-001~003）
│  ├─ queries/                 #   sqlc 的 .sql 輸入
│  └─ sqlc.yaml                #   產物輸出至 services/platform/internal/platform/db/gen/
├─ spikes/                     # 既有，保留。M0 探索用，不進 CI、不被產品程式碼 import
├─ plans/                      # 既有。MVP 目標／規格／工作清單
└─ adr/                        # 既有。架構決策紀錄
```

**M1 只建立標註 M1 的目錄。** `services/sandbox/`、`contracts/events/` 等到對應里程碑再建，空目錄不預先佔位。

**鐵律 2 的結構性保證**：`services/sandbox/` 使用**獨立的 Go module**（自己的 `go.mod`），而非 `services/platform` 底下的另一個 `cmd/`。搭配 Go 的 `internal/` 語意，執行平面在編譯期就無法 import 控制平面的資料存取 package——不需要額外的 lint 設定就能擋住鐵律 2 的違反。控制平面內部的模組依賴檢查另依 ADR-016（Go `internal` package ＋ 依賴 lint）。

**產物入 repo 的理由**：`packages/` 與 sqlc 輸出都 commit 進 repo。這讓「重跑 codegen 後 `git diff` 乾淨」成為一個不需要任何基礎設施的 drift 檢查（第 3 節 job 4），也讓 IDE 與 code review 看得到實際介面。代價是 diff 噪音，可接受。

### 2. 工具鏈

| 範圍     | 工具                                                      | 說明                  |
| -------- | --------------------------------------------------------- | --------------------- |
| Go       | `go build` / `go test` / `golangci-lint`                  | 不引入額外建置層      |
| TS       | Vite ＋ `tsc` ＋ Oxlint ＋ Prettier ＋ Vitest             | 前端原生              |
| Python   | `uv` ＋ `ruff` ＋ `pytest`                                | 依 ADR-016 用 uv 管理 |
| 資料庫   | Migration 工具 ＋ `sqlc generate`                         | 產物入 repo           |
| 契約     | OpenAPI codegen（Go 為來源，產 TS client 與 Python stub） | 產物入 repo           |
| 共同入口 | **Taskfile**（`Taskfile.yml`）                            | 見下                  |

共同入口選 **Taskfile 而非 Makefile**：目前開發環境為 Windows，`make` 需要額外安裝 MSYS/GNU make 才能用，而 Taskfile 是單一跨平台執行檔（與 Go 工具鏈同生態）。它的職責只有「把常用指令記在一個地方」，**不得**演變成依賴圖或快取層——需要那個能力時，走上方「重評條件」重開評估。

最小 task 集合（每個都只是包一行原生指令）：`dev`（compose 起全部）、`test`、`lint`、`gen`（跑 codegen ＋ sqlc）、`build`。

### 3. CI/CD（GitHub Actions）

觸發：對 `main` 的 PR，以及 push 到 `main`。所有 job 在 PR 上都必須綠燈。

| #   | Job               | Path filter                                                  | 內容                                                                                               |
| --- | ----------------- | ------------------------------------------------------------ | -------------------------------------------------------------------------------------------------- |
| 1   | `web`             | `apps/web/**`、`packages/api-client-ts/**`                   | install → lint → typecheck → test → `vite build`                                                   |
| 2   | `platform`        | `services/platform/**`、`db/**`、`contracts/**`              | `golangci-lint` → `go test ./...` → `go build ./...`                                               |
| 3   | `llm`             | `services/llm/**`、`packages/api-stub-py/**`、`contracts/**` | `uv sync --frozen` → `ruff check` → `pytest`                                                       |
| 4   | `contracts-drift` | **無 filter，每次都跑**                                      | 重跑 codegen 與 `sqlc generate`，接著 `git diff --exit-code`；有 diff 即失敗（鐵律 12）            |
| 5   | `images`          | 同 1～3，但**只在 push 到 `main`**                           | build `apps/web`、`services/platform`、`services/llm` 的容器 Image，tag 為 commit SHA，推 registry |

設計說明：

- Job 4 **刻意不做 path filter**。drift 也可能來自「有人直接手改產物」，而那種 commit 不會碰到 `contracts/`。這個 job 很便宜，全跑最省事。
- Job 5 在 PR 上**不執行**（PR 的建置正確性由 1～3 保證，不需要為每個 PR 推 Image）。Image tag 用 commit SHA 而非 `latest`，讓部署與回滾都指向明確的 commit。
- `spikes/**` 在所有 job 的 path filter 中排除；spike 程式碼不進產品建置。
- 只改 `plans/**`、`adr/**`、`*.md` 的 PR 只會跑 job 4，數十秒完成。

**main 分支保護建議**：禁止直接 push；合併需通過上述 required checks；要求 linear history（squash merge）；PR review 的強制程度見「待決策」。

**部署（方向，不展開）**：job 5 產出的 Image 由 E1 的 compose 拉取（`docker compose pull` ＋ 重啟受影響服務），初期以手動觸發（`workflow_dispatch`）而非自動推送到生產。環境晉升、資料 Migration 的執行時機與 Rollback 策略是 ADR-010 的公開待決策，**依 ADR-018 定案後補**，本 ADR 不預設。

### 4. 慣例落地

- **語言**：程式碼、識別字、註解、commit message、CI job 名稱一律英文；`plans/`、`adr/`、面向使用者的文件用繁體中文（AGENTS.md 慣例）。commit message 用英文祈使句一行摘要，暫不強制 Conventional Commits，CI 不檢查。
- **Migration**：`db/migrations/` 內的檔案已套用後**不可修改**——修正等於新增一份 Migration（與 ADR-003 的不可變原則一致）。Migration 與需要它的程式碼在同一個 PR。
- **sqlc**：查詢寫在 `db/queries/*.sql`，`sqlc.yaml` 與 Migration 同目錄（schema 與查詢同源），產物輸出到 Go module 內並入 repo，由 job 4 檢 drift。
- **Secrets 與設定**：現有 `.gitignore` 已排除 `.env` 與 `.venv/`，維持不變並補上 `node_modules/`、`dist/`、Go 建置輸出。repo 內只放 `.env.example`（僅變數名稱與明顯的假值），實際值由容器環境變數與 GitHub Actions 的加密設定注入（鐵律 11、NFR-002）。任何 CI log 與建置產物不得回印這些值。

## 影響

### 正面

- M1 可以立即開工：CORE／INGEST／DISC／WS 四組工作項都有明確的落點。
- 鐵律 12 的 drift 檢查是一個 `git diff`，不需要維護額外基礎設施。
- 鐵律 2 由 Go module 邊界在編譯期保證，不依賴人工審查。
- ADR-010 的服務拆分接縫保留：`services/` 底下每個目錄本來就是獨立的建置與 Image 產物，拆成獨立部署不需要搬程式碼。

### 成本與限制

- Path filter 是手寫的，新增頂層目錄時必須同步更新 workflow，否則會有「該跑卻沒跑」的靜默漏洞。這是選項 D 明知的代價。
- codegen 產物入 repo 會讓部分 PR 的 diff 變大。
- 單一 repo 隨 M2～M4 成長後，全量 checkout 與 IDE 索引成本會上升；出現實際痛點前不處理。
- CI 目前不涵蓋 Sandbox 相關測試（SBX-010、SEC-009）——那需要巢狀虛擬化能力，見待決策。

## 待決策

1. ~~**Container registry**：GHCR（與 Actions 整合最省事）、部署平台自帶的 registry，或其他？~~ → **已回填（2026-08-16）：採 GHCR**，見 [ADR-022](./ADR-022-sandbox-deployment-topology-and-security-thresholds.md)「補充決策：Container registry」。決策驅動因素不只是 CI 整合，而是 **SEC-002 的 I-03／I-04 需要 SBOM 與掃描 attestation 隨 image digest 保存**（OCI referrer／`cosign attach`）才能在閘門 A 查得到；沒有這個落點，SEC-009 的「0 unknown」結構性不可達。發佈流水線接 GHCR push ＋ attestation 為實作工作（`03` 第 11 節 SBX-011）。
2. **Branch 與 review 策略**：trunk-based（`main` ＋ 短命 feature branch）是預設提案；單人開發期間是否要求 PR 至少一個 approval，或允許自行合併但保留 required checks？
3. **CI runner**：GitHub-hosted runner 足以跑 job 1～5，但 M2 的 gVisor／`runsc` 隔離測試（SBX-010、SEC-009）需要巢狀虛擬化，可能得改用 self-hosted runner。要在 M2 前確認，或接受該類測試只在部署平台的節點上手動執行？→ **範圍已縮小（2026-08-16）**：[ADR-022](./ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 第三部分 §0 指出 **`runsc` 的預設 `systrap` 平台不需巢狀虛擬化**（只有 `--platform=kvm` 才需要 `/dev/kvm`），因此計算隔離、資源耗盡、Runtime 相容性與映像供應鏈測項（Suite 1）在 `ubuntu-latest` 即可跑；只有節點與網路面（Suite 2）需要真實節點。**本問題因此不再需要 self-hosted runner，但該前提須由部署批的第一台節點實測確認後才視為回答。**
4. **前端套件管理器**：pnpm 或 npm（影響 lockfile 與 CI 快取設定，無架構後果，指定即可）。
5. **`spikes/` 的長期處置**：M1 後保留為歷史紀錄，或在 M0 文件完成引用後移除？目前提案是保留且不進 CI。
