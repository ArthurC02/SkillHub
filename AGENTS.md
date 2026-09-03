# Skill Hub — Coding Agent 導覽

## 這是什麼專案

Skill Hub 是 Agent Skill 的搜尋引擎與試驗室：個人創作者以自然語言描述任務 → 探索候選 Skill → 用自己的 Prompt 與測試資料在隔離 Sandbox 試跑 → 依 Trace 與評估報告改善 → 下載符合 Agent Skills 規格的可攜套件。

## 系統怎麼跑

一個請求的路徑，四個行程：

1. **`apps/web`**（React SPA）只對 `apps/platform` 的 API 說話；契約在 `contracts/openapi/public.yaml`（OpenAPI-first，Go 是下游）。
2. **`apps/platform`**（Go）是唯一的控制平面：API 行程做 AuthZ 與領域規則；Run 狀態機、River 佇列、FTS＋pgvector、Trace 分割表全在同一個 PostgreSQL；物件儲存講 S3 協定（本機 SeaweedFS）。
3. **Go Worker** 是唯一的佇列消費者，以內部 HTTP 呼叫兩個能力提供者：**`apps/llm`**（Python FastAPI，所有模型呼叫經 LiteLLM 閘道）與 **`apps/sandbox`**（gVisor 隔離的 VM 池，跑釘選 digest 的 runtime image；契約 `contracts/openapi/sandbox-provider.yaml`）。
4. 結果以結構化回應回到 Go；領域狀態變更與對外事件同一交易（outbox）。**執行平面永遠碰不到核心資料庫。**

`apps/platform/internal/` 依 DDD 切成 creator／product／skill／trial 四群 Bounded Context，逐套件對照表在 ADR-032 §1（有機器對帳）。架構總圖見 [docs/adr/README.md](docs/adr/README.md)。

## 現在在哪

M0～M6 的程式面皆已收斂；**「程式面收斂」不等於「完成」**——M1 驗證閘門、M4 封測、M5 曝光都在等真人數字。**新功能凍結生效中**，逐次放行紀錄、閘門期間條款與三條 ⛔ 邊界只在 [`01` §10](docs/plans/01-goals-and-plan.md)；其中最硬的一條：**M5 的生成入口不得對封測使用者出現**。狀態變動改 `01` §10 與 `04`，不改這裡——本檔不放日期、不放數字。

## 目錄地圖

| 目錄 | 內容 |
| --- | --- |
| `apps/` | 可獨立啟動／建置／部署的產品程式：`web`、`platform`、`llm`、`sandbox` |
| `packages/` | 供其他程式 import 的 library；generated 子目錄禁止手改 |
| `contracts/` | 跨程序／跨語言介面的唯一來源 |
| `db/` | Migration、query、sqlc 設定與 `query-owners.yaml` |
| `infra/` | 部署、runtime image、網路、節點與 observability |
| `tools/` | 開發、CI、資料維護、維運命令；`tools/devctl` 是所有機器檢查的家 |
| `docs/plans/` | 產品基準：`01` 目標與里程碑、`02` 規格允收（需求 ID）、`03` 工作清單、`04` 殘項與移交、`05` 待裁定；`mvp/mX/` 為凍結產出 |
| `docs/adr/` | 架構決策；份數、狀態與取代關係見 [索引](docs/adr/README.md) |
| `docs/design/` | 前端兩把尺：[system.md](docs/design/system.md) 管一頁之內、[information-architecture.md](docs/design/information-architecture.md) 管頁與頁之間；**兩份都有機器測試直接解析** |
| `docs/development/` | 開工前讀的手冊；[docs/runbooks/](docs/runbooks/) 是出事時讀的 |

收納語意由 ADR-031 定義，CI/CD 基線見 ADR-019；結構性偏離先更新 ADR。

## 技術棧

| 層 | 選擇 | 依據 |
| --- | --- | --- |
| 前端 | React + TS（Vite、TanStack Router/Query） | ADR-016 |
| 平台後端 | Go：薄 HTTP 層、pgx + sqlc、River | ADR-016、018 |
| LLM 工作負載 | Python FastAPI（uv），內部服務 | ADR-016 |
| 模型供應商 | OpenAI（試跑預設 mini 級；Embedding `text-embedding-3-small`），一律經 LiteLLM，每 Run 短效 Virtual Key | ADR-017 |
| 資料 | PostgreSQL 中心 ＋ S3 相容物件儲存；核心元件容器化自架 | ADR-018 |
| 搜尋 | 混合檢索（向量腿 ＋ FTS 腿 `UNION` 擴充候選，不做 RRF）＋ 索引時 LLM 增強 | ADR-013 |
| Agent Runtime | Claude Agent SDK，事實來源是 image digest；版本字串釘在 `infra/images/runtime-agent-sdk/Dockerfile` 的 `ARG`，**不在 `tools/toolchain.yaml`**；升級必重跑四項實測 | ADR-023 |
| 身分 | GitHub OAuth ＋ Postgres Session（`DEV_LOGIN` 為離線 provider） | ADR-020 |
| Sandbox | gVisor `systrap`，獨立 VM 池，nftables default-deny ＋固定 DNS，不部署 L7 Proxy | ADR-015、005、022 |
| Runtime Image | 自建映像發佈至 GHCR，SBOM 與掃描以 attestation 隨 digest 保存 | ADR-022 |
| LLM 觀測 | Langfuse Cloud（工程調優用，非事實來源；**MVP 未實作**，見 [`05` R-24](docs/plans/05-pending-rulings.md)） | ADR-017 |
| 契約 | OpenAPI-first；Go 側 models-only，handler 手寫並逐條對齊 | ADR-016、030 |

**Local Runner 與遠端 MCP 已移出 MVP 首發**（決策保留於 ADR-006）。

## 實作鐵律（違反任何一條 = 架構回歸）

1. 不受信任的 Skill、Script、資料不得在 Web/API 程序內執行；匯入與掃描階段不得執行套件內 Script。（ADR-001、007）
2. 執行平面不得直接存取核心資料庫；只透過任務契約、短效物件授權與事件互動。（ADR-001）
3. 所有使用者資料查詢預設要求 Workspace Scope；不信任 UI 傳入的 `workspace_id`。（ADR-011）
4. Skill Version、Test Case 快照、歷史 Run 不可變；採用改善建議＝建立新版本，不原地覆寫。（ADR-003）
5. Run 狀態的唯一事實來源是 Go 擁有的 Postgres 狀態機；Python 側的任何程序內狀態都是暫存草稿，不得回寫成狀態。（ADR-008、016）
6. Python 是能力提供者：收結構化請求、回結構化結果；政策、授權、狀態轉移、重試決策全在 Go，業務規則不進 Python。（ADR-016）
7. 佇列消費者只有 Go Worker；Python 不消費佇列，由 Go 以內部 HTTP 呼叫（含逾時與取消傳遞）。（ADR-016）
8. 所有模型呼叫走 LiteLLM 閘道，不得直連供應商；供應商金鑰只存在閘道。（ADR-017）
9. 領域狀態變更與對外事件同交易（Transactional Outbox）；Consumer 必須冪等；`destroy`/清理可安全重複。（ADR-008、004）
10. 平台 `run_id` 是永久識別；Provider 臨時 ID 不得當主鍵或永久 URL。（ADR-004）
11. Secrets 不得出現在套件、Log、Trace 明文或分析事件；顯示前完成遮罩。（NFR-002、TRACE-001）
12. 跨語言介面先寫 OpenAPI schema 再實作；CI 以 codegen 檢查 drift。（ADR-016）

## 文件維護規則

- 三份 MVP 文件（目標／規格／工作清單）改範圍時必須同步；規格新功能先補需求 ID 與允收準則。
- 改 `04` 的殘項數字時，同一格末尾的 `<!-- open: … -->` 要一起改（`backlog-tally` 會對帳）。
- `- [ ]` → `- [x]` 只在完全符合允收準則時；部分完成保持未勾。
- ADR 是決策歷史：推翻＝新增 ADR 並把舊的標 `Superseded`，不刪除、不原地改寫；下一號＝[索引](docs/adr/README.md)最大號 + 1，新增後更新索引。
- 活文件放 `docs/plans/` 根層；里程碑產出放 `docs/plans/mvp/mX/`，完結即凍結，不回溯修正（含 `03` 的歷史 flat path）。
- 里程碑目錄固定骨架：`README.md`（計畫＋狀態＋檔案地圖）、`audit.md`、報告用 `report-*` 前綴；目錄內檔名不重複 `mX` 前綴（M3 起適用，既有檔名不回溯改）。

## 慣例

- 文件：繁體中文（Run、Workspace、Provider 等術語不硬翻）。程式碼、識別字、commit message：英文。
- **每次回覆的最後一段，是給沒讀過程式碼的人看的白話摘要**：一段連貫敘述講「做了什麼、發現了什麼、還缺什麼」，不條列、不堆檔名。理由：看不懂的綠燈與看不懂的紅燈一樣沒有用。
- 需求 ID 前綴：DISC／SKILL／WS／TEST／RUN／SBX／TRACE／EVAL／PACK／GEN／PORT／NFR／PDM／SEC，見 `02`、`03`（各自編號）。

## 開發自動化（Agent 開工先讀）

完整操作、成本分級、生成來源表與每條紅線的來歷以 [docs/development/automation.md](docs/development/automation.md) 為準；本節只放紅線。不要把只在某一台電腦成立的 native command 當成 repo 的標準流程。

1. **先診斷再修改**：`task --list`，再 `task doctor`（未裝 Task 時 `go -C tools/devctl run . doctor`）。版本來源是各語言原生檔與 `tools/toolchain.yaml`；doctor 的版本不符是診斷結果，不得靠跳過檢查偽裝成通過。
2. **預設不花錢**：`task dev` 只起 Postgres 與 SeaweedFS。`task dev:model` 會產生費用，**不得由唯讀 SubAgent 自行啟動**。
3. **共享工作樹、單一 Writer**：SubAgent 預設唯讀，同一時間只能有一個 Writer；寫入 SubAgent 必須有精確 path allowlist，不得自行執行 repo-wide formatter、package install、Compose down、Git 寫入或 lockfile 更新。**禁止 `git stash`**（會連他人未提交與未追蹤的工作一起收走，本專案已三度因此出事）；對未知修改同樣禁止 `git reset`、`git clean`、`git checkout -- <path>`——看到不屬於自己的 delta 就保留並回報。只以明確 pathspec stage 自己的檔案、push 前 `git pull --rebase`；暫存產物放 scratchpad。**子代理的模型**：每次派工明確指定，**禁止 Fable（Claude）與 Sol（Codex）**；依任務難度從最低階起、升級要有理由——階梯與各工具的對應表在 automation.md〈共享工作樹與 SubAgent〉。
4. **高衝突區由主 Agent 序列化**：`contracts/`、`db/migrations/`、`db/queries/`、generated 目錄、`go.sum`／`package-lock.json`／`uv.lock`、`Taskfile.yml` 與 `.github/workflows/`。
5. **generated files 禁止手改**：`task gen:sql`／`task gen:openapi` 由主 Agent 序列化執行；提交前一律 `task gen:check`；generated 目錄的衝突在來源解決後重生。
6. **Go generated router 不擁有 AuthZ**：ogen server 只在 `router.go` 的精確 `GET /healthz` pattern 後；其他 route 逐條套 `RequireSession`／`RequireOperator`／`OptionalSession`，不得整批 mount，每移一條要加 route 測試。
7. **Bounded Context 治理（ADR-032）**：每個套件屬於且僅屬於一個 context，新套件先在 ADR-032 §1 登記再建目錄；跨 context 的新 import **同一個 commit** 改附錄 A 與 `apps/platform/.golangci.yml` 的 depguard；領域 Service 只由 `entrypoint/api/apiserver.NewApp` 注入，禁止方法內現場建構。日常判斷見 [platform-ddd-practices.md](docs/development/platform-ddd-practices.md)。
8. **Query ownership（ADR-033、035）**：owner 宣告在 `db/query-owners.yaml`，跨 context 呼叫會 FAIL；新增或刪除 query 同一批改該檔；`allow:`／`read_allow:` 是存量漂移清單，**不是擴充點**。
9. **修好一個東西之後，把修法弄壞一次**：綠燈只證明測試存在。把修正那一行還原、跑對應測試、確認變紅、再改回來（`git diff` 為空）。不適用純文案與純註解；適用任何你在 commit 訊息裡寫「修好了 X」的東西——那句話的證據就是那次紅。三次前例在 automation.md。

## 分區指標與攔阻

**動某個區域之前，先讀那個區域的 `AGENTS.md`**——它列出「你要做的事 → 先讀哪一段 → 沒讀會被哪個閘門擋」：

| 你要動 | 先讀 |
| --- | --- |
| `apps/web/` | [`apps/web/AGENTS.md`](apps/web/AGENTS.md) |
| `apps/platform/internal/` | [`apps/platform/internal/AGENTS.md`](apps/platform/internal/AGENTS.md) ＋ 目標套件的 `doc.go` |

這張表是唯一保證送達的入口（目錄層 `AGENTS.md` 各工具送達方式不一）。Claude Code 另有 `.claude/rules/`（按路徑送指標）、`.claude/agents/`（按風險切的三個角色：writer／verify／mutation）、`.claude/skills/`（換一個 repo 還成立的程序）與 `permissions.deny`；**它們不取代文件，只是提早發現**，放置規則、新增區域配方與守它們的機器見 automation.md〈Harness〉。

## 去哪看

| 你要做的事 | 先看 |
| --- | --- |
| 狀態全貌、里程碑、凍結放行與 ⛔ 邊界 | [`01` §10](docs/plans/01-goals-and-plan.md) |
| 還缺什麼、誰在等誰（殘項總數唯一來源） | [`04`](docs/plans/04-backlog-and-handoffs.md) |
| 我現在要簽什麼 | [`05`](docs/plans/05-pending-rulings.md) |
| 某功能的允收準則 | [`02`](docs/plans/02-specifications-and-acceptance-criteria.md)（按需求 ID） |
| 下一個工作項目 | [`03`](docs/plans/03-work-items.md) |
| 系統邊界與平面／資料模型／Run 生命週期／安全 | ADR-001、002／003、018／004、008／005、007、015、022 |
| Query 屬於誰、跨 context 怎麼拿事實 | ADR-033、035、034 ＋ [platform-ddd-practices.md](docs/development/platform-ddd-practices.md)；動手前讀目標套件的 `doc.go` |
| 派送被停了怎麼判斷與解除 | [p1-dispatch-halt.md](docs/runbooks/p1-dispatch-halt.md) |
| 畫面該長什麼樣、新畫面放哪個網址 | [system.md](docs/design/system.md) §3、[information-architecture.md](docs/design/information-architecture.md) §0（先過這關再寫） |
| 封測上線前要做什麼 | [release-checklist](docs/plans/mvp/m4/release-checklist.md) |
| 同意書與資料保存 | [consent-and-data-policy.md](docs/plans/mvp/gate-test/consent-and-data-policy.md) |
| 評估判定與 Judge／打包簽章／封測准入／漏斗量測 | ADR-025、026／027／028／029 |
| 所有未決議題 | 各 ADR 的「待決策」＋ `03` 第 1 節 |
