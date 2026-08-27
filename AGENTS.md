# Skill Hub — Coding Agent 導覽

## 這是什麼專案

Skill Hub 是 Agent Skill 的搜尋引擎與試驗室：個人創作者以自然語言描述任務 → 探索候選 Skill → 用自己的 Prompt 與測試資料在隔離 Sandbox 試跑 → 依 Trace 與評估報告改善 → 下載符合 Agent Skills 規格的可攜套件。

## 目前狀態

**M0～M4 的程式面已收斂，M5 的程式面亦已收斂**（**◐ 的只剩 `GEN-009`（`GEN-008` 已於 2026-08-25 改判勾選，本句 2026-08-26 才跟上）；勾選數以 [`03` §19](docs/plans/03-work-items.md) 的 checkbox 為準，本檔不複述**——那個數字曾經同時存在於五份文件並三度彼此不符）。**「程式面收斂」不等於「MVP 完成」——剩下的是部署期與負責人動作，不是程式。** M4 與 M5 同時未完結（[ADR-052](docs/adr/ADR-052-m5-starts-in-parallel-with-an-unfinished-mvp.md) 明示接受）。

**⛔ 硬邊界：開工不等於曝光**——M5 的生成入口不得對封測使用者出現，直到漏斗第一段有讀數為止。三條仍然生效的邊界逐條見 `01` §10。

| 你要問的 | 唯一來源 |
| --- | --- |
| 狀態全貌、里程碑表與三條 ⛔ 邊界 | [`01` §10 里程碑](docs/plans/01-goals-and-plan.md) |
| 還缺什麼、誰在等誰（**殘項總數的唯一來源**） | [`04` 現況一覽](docs/plans/04-backlog-and-handoffs.md)（活文件） |
| 我現在要簽什麼 | [`05` 待裁定清單](docs/plans/05-pending-rulings.md)（活文件） |

狀態變動改 `01` §10 與 `04`，不改這裡。

| 目錄 | 內容 | 入口 |
| --- | --- | --- |
| `docs/plans/` | [產品基準](docs/plans/README.md)：目標、規格允收準則（需求 ID）、工作清單、[殘項與移交](docs/plans/04-backlog-and-handoffs.md)（活文件）、[待裁定清單](docs/plans/05-pending-rulings.md)（活文件）；`mvp/m0/`～`mvp/m5/` 為各里程碑凍結產出；`mvp/content/`／`mvp/governance/`／`mvp/gate-test/` 為跨里程碑仍在被引用的主題目錄（ADR-031） | [docs/plans/README.md](docs/plans/README.md) |
| `docs/adr/` | 架構決策紀錄。**份數、狀態與取代關係見索引** | [docs/adr/README.md](docs/adr/README.md)（含索引與架構總圖） |
| `docs/spikes/` | M0 spike code 已刪，只留墓碑與結論落點對照 | [docs/spikes/README.md](docs/spikes/README.md) |
| `docs/design/` | **前端的兩把尺**：[system.md](docs/design/system.md) 管**一頁之內**（義務、原則、字級／間距／表面／狀態語彙、強制對照表），[information-architecture.md](docs/design/information-architecture.md) 管**一頁與一頁之間**（規則層 R1～R7、路由、導覽、可達性、網址狀態、旗標入口）。兩份都是活文件且各有機器測試。**注意 IA 的方向**：§0 的規則走在程式前面（不一致改程式），§1～§4 的盤點跟在程式後面 | [docs/design/system.md](docs/design/system.md)、[information-architecture.md](docs/design/information-architecture.md) |
| `docs/runbooks/` | 值班當下照著做的操作程序（`02:SEC-010` 要求的形式）。與 `docs/development/` 的分別：那裡是開工前讀的手冊，這裡是出事時讀的 | [p1-dispatch-halt.md](docs/runbooks/p1-dispatch-halt.md)（P1 停止派送與解除） |

Monorepo 的 CI/CD 基線見 **ADR-019**，頂層收納由 **ADR-031（Accepted）** 按產物角色定義；結構性偏離需先更新 ADR。

| 頂層目錄 | 收納語意 |
| --- | --- |
| `apps/` | 可獨立啟動、建置或部署的產品程式：`web`、`platform`、`llm`、`sandbox` |
| `packages/` | 供其他程式 import 的 library；generated 子目錄禁止手改 |
| `contracts/` | 跨程序／跨語言介面的唯一來源 |
| `db/` | Migration、query、sqlc config 與 DB test 等持久化來源 |
| `infra/` | 部署、runtime image、網路、節點與 observability 設定 |
| `tools/` | 開發、CI、資料維護、維運命令及其緊密 fixture |
| `docs/` | 不被產品程式 import 或執行的敘事與歷史文件 |

`.github/`、`.devcontainer/` 與 repo 根層工具入口保留在平台預期位置。不是所有含程式碼的內容都進 `apps/`，判準是能否作為產品程式獨立啟動／建置／部署。

## 已定案的技術棧速覽

| 層 | 選擇 | 依據 |
| --- | --- | --- |
| 前端 | React + TS（Vite、TanStack Router/Query），SPA 起步 | ADR-016 |
| 平台後端 | Go：chi/echo 薄層、pgx + sqlc、River（Postgres 佇列） | ADR-016、018（ADR-014 已 Superseded，核心結論由 018 延續） |
| LLM 工作負載 | Python：FastAPI（uv 管理），內部服務（未採用 LangGraph） | ADR-016 |
| 模型供應商 | OpenAI API（試跑預設 mini 級；Embedding `text-embedding-3-small`），一律經 LiteLLM 閘道 | PDM-003、ADR-017 |
| 資料 | PostgreSQL 中心（交易、FTS + pgvector、佇列、Trace 分割表）＋受管 S3 相容物件儲存；核心元件容器化自架（E1） | ADR-018 |
| 搜尋 | 混合檢索（向量腿承載跨語言召回，FTS＋RRF 為召回覆蓋）＋索引時 LLM 增強（摘要與任務範例句為必要項） | ADR-013 |
| Agent Runtime | Claude Agent SDK，版本以 digest ＋ lockfile 釘選，**最終事實來源是 image digest**（ADR-023 決策 1；版本字串釘在 `infra/images/runtime-agent-sdk/Dockerfile` 的 `ARG CLAUDE_AGENT_SDK_VERSION`，**不在 `tools/toolchain.yaml`**——那個檔沒有這一項）；升級必須重跑四項實測，靜默失效不得以推理帶過 | ADR-023 |
| 身分與 Session | GitHub OAuth ＋ Postgres Session（`DEV_LOGIN` 為離線 provider） | ADR-020 |
| Sandbox 隔離 | gVisor 基線（`systrap` 平台，不需巢狀虛擬化），獨立 VM 池，沙箱層 nftables default-deny ＋固定 DNS（不部署 L7 Proxy） | ADR-015、005、022 |
| Runtime Image | 自建映像發佈至 **GHCR**，SBOM 與漏洞掃描以 attestation 隨 digest 保存；過不了門檻的映像到不了 registry | ADR-022、`03` SBX-011 |
| 模型出口 | LiteLLM Proxy（唯一模型閘道，每 Run 短效 Virtual Key） | ADR-017 |
| LLM 觀測 | Langfuse Cloud（工程調優專用，非事實來源） | ADR-017 |
| 契約 | OpenAPI-first，Go 為 spec 來源，codegen 產 TS/Python stub | ADR-016 |

範圍注意：**Local Runner 與遠端 MCP 已移出 MVP 首發**（架構決策保留於 ADR-006 與相關規格，實作依需求訊號啟動）。

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
- **改 `04` 的殘項數字時，同一格末尾的 `<!-- open: … -->` 要一起改**——`devctl automation-check` 的 `backlog-tally` 比對數字、清單與實際的列，漏了 CI 會 FAIL。
- 工作項目 `- [ ]` → `- [x]` 只在完全符合允收準則時；部分完成保持未勾。
- ADR 是決策歷史：推翻舊決策＝新增 ADR 並把舊的標 `Superseded`，不刪除、不原地改寫決策內容；待決策被後續 ADR 回答時回填 `→ [ADR-xxx](...)` 引用。
- **下一號 ADR ＝ [docs/adr/README.md](docs/adr/README.md) 索引的最大號 + 1**，新增後記得更新該索引。選型類決策採 ADR-016 格式（含「評估選項」比較），邊界類可用精簡格式。
- **檔案放哪裡**：活文件放 `docs/plans/` 根層（編號 `01~`）；里程碑的歷史產出放 `docs/plans/mvp/mX/`，里程碑完結即凍結。跨里程碑主題材料放 `docs/plans/mvp/` 的相應主題目錄。一份文件如果會被下一個里程碑繼續改，它就不屬於 `mX/`。
- **里程碑目錄固定骨架**：`README.md`（計畫＋狀態＋檔案地圖）、`audit.md`、報告用 `report-*` 前綴；目錄內檔名不重複 `mX` 前綴。**M3 起適用，既有檔名不回溯改。**
- **`03` 的歷史完成紀錄保留當時的 flat path**——那是里程碑時點的證據，不要順手修正。

## 慣例

- 文件語言：繁體中文（保留 Run、Workspace、Provider 等英文術語不硬翻）。程式碼、識別字、commit message：英文。
- **每次回覆的最後一段，是給沒有讀過這份程式碼的人看的白話摘要**：一段連貫的敘述，講「做了什麼、發現了什麼、還缺什麼」，可以用比喻，**不要條列破碎的技術片段、不要堆檔名與識別字**。技術細節寫在它前面或寫進文件裡；那一段本身要能單獨被讀懂。<br>**理由**：本專案的判斷全靠「這件事到底成立到什麼程度」，而一份讀不懂的摘要會讓負責人只能相信結論——**看不懂的綠燈與看不懂的紅燈一樣沒有用**。
- 多人／多 agent 共用同一工作樹平行作業時：只以明確 pathspec stage 自己的檔案、push 前 `git pull --rebase`；**禁止 `git stash`**（stash 會連他人未提交與未追蹤的工作一起收走，本專案已三度因此出事）；暫存產物放 scratchpad，不放 repo 根目錄。
- 里程碑：M0 基線 → M1 Explorer（結尾有驗證閘門，不通過不進 M2）→ M2 Lab → M3 評估 → M4 打包與封測 → M5 從任務描述生成 Skill（ADR-046／052／054）。**M5 不在 MVP 完成度內**（`01` §7.3）；各里程碑現況見 `01` §10。
- 需求 ID 前綴：DISC／SKILL／WS／TEST／RUN／SBX／TRACE／EVAL／PACK／GEN／NFR／PDM／SEC 等，見 `docs/plans/02`、`03`。（`02` 與 `03` 各自編號，同前綴不同號。）

## 開發自動化（Agent 開工先讀）

決策見 [ADR-030](docs/adr/ADR-030-portable-developer-automation-and-contract-code-generation.md)；**完整操作、能力與成本分級、生成來源表與排錯以 [開發自動化手冊](docs/development/automation.md) 為準**（本節只放紅線）；Platform 的 Bounded Context 日常判斷見 [DDD 實務指南](docs/development/platform-ddd-practices.md)。不要把只在某一台電腦成立的 native command 當成 repo 的標準流程。

1. **先診斷再修改**：進入 repo 後先看 `task --list`，再跑 `task doctor`（尚未安裝 Task 時用 `go -C tools/devctl run . doctor`）。版本來源是各語言原生檔與 `tools/toolchain.yaml`，不是 Agent 記憶；doctor 的版本不符是環境診斷，不得靠跳過檢查偽裝成通過。
2. **預設不花錢**：`task dev`／`dev:core` 只起 Postgres 與 SeaweedFS。`task dev:model` 會 fail-closed 檢查模型秘密且後續會產生費用，**不得由唯讀 SubAgent 自行啟動**；完整能力與成本分級表見 automation.md。
3. **共享工作樹、單一 Writer**：SubAgent 預設唯讀，同一時間只能有一個 Writer；寫入 SubAgent 必須有精確 path allowlist，不得自行執行 repo-wide formatter、package install、Compose down、Git 寫入或 lockfile 更新。除禁止 `git stash` 外，對未知修改同樣禁止 `git reset`、`git clean`、`git checkout -- <path>`——看到不屬於自己的 delta 就保留並回報。
4. **高衝突區由主 Agent 序列化**：`contracts/`、`db/migrations/`、`db/queries/`、generated 目錄、`go.sum`／`package-lock.json`／`uv.lock`、`Taskfile.yml` 與 `.github/workflows/` 不交給多個寫入 Agent 平行處理。
5. **generated files 禁止手改**：`task gen:sql`／`task gen:openapi` 由主 Agent 序列化執行，SubAgent 不自行執行；提交前一律跑 `task gen:check`。generated 目錄的衝突要在來源解決後重生，不得手動合併。人工來源與生成目標的對照表在 automation.md。
6. **Go generated router 不擁有 AuthZ**：ogen server 只在 `router.go` 的精確 `GET /healthz` pattern 後；其他 route 逐條由 `router.go` 套 `RequireSession`／`RequireOperator`／`OptionalSession`，不得整批 mount generated server。每移一條 endpoint 都要保留原 middleware 語意並加 route 測試（順序見 automation.md）。
7. **Platform 的 Bounded Context 治理（ADR-032）**：`apps/platform/internal/` 每個套件屬於且僅屬於一個 context；新增套件必須先在 ADR-032 §1 對照表登記。跨 context 的新 import 必須**同一個 commit** 改 ADR-032 附錄 A 與 `apps/platform/.golangci.yml` 的 depguard 規則（CI 以 depguard ＋ `devctl automation-check` 兩道強制）；領域 Service 一律由 `entrypoint/api/apiserver.NewApp` 注入，禁止方法內現場建構其他 context 的 Service。**逐套件對照表以 ADR-032 §1 為準，受 `devctl automation-check` 機械對帳**；日常判斷見 [platform-ddd-practices.md](docs/development/platform-ddd-practices.md)。
8. **Query ownership（ADR-033、035）**：每條 query 的 owner context 宣告在 `db/query-owners.yaml`，由 `devctl automation-check` 強制——**跨 context 的 write query 呼叫會 FAIL**（read 已一併強制）。新增或刪除 `db/queries/*.sql` 的 query，同一批要改 `db/query-owners.yaml`，漏了 CI 會 FAIL；該檔的 `allow:`／`read_allow:` 是**存量漂移清單，不是擴充點**——新的跨 context 存取要改程式，不准往下面加行。
9. **修好一個東西之後，把修法弄壞一次**：改完跑一次測試是綠的，那只證明測試存在，不證明它會紅。**把修正那一行還原、跑對應的測試、確認它變紅、再改回來**（`git diff` 確認為空）。本專案已經三次「修好了但測試沒有牙齒」：invalidate 用錯 query key（改回錯的、171 支全綠）、placeholder 六條規則有三條零正面測試（同時刪掉、全綠）、兩條 import 路由的速率限制包裝（無聲刪掉、全套綠）。**三次都不是 code review 找到的**，是突變找到的。成本是一次 `sed` 加一次 `go test`。<br>**不適用於**：純文案、純註解、以及本來就沒有斷言可言的變更。**適用於**：任何你在 commit 訊息裡寫「修好了 X」的東西——那句話的證據就是那次紅。

## 快速判斷「我該看哪份文件」

| 你要做的事 | 先看 |
| --- | --- |
| 理解產品範圍與里程碑 | `docs/plans/01-goals-and-plan.md` |
| 查某功能的允收準則 | `docs/plans/02-specifications-and-acceptance-criteria.md`（按需求 ID） |
| 找下一個工作項目 | `docs/plans/03-work-items.md`（章節已標里程碑） |
| 理解系統邊界與平面 | ADR-001、002 |
| 資料模型與儲存 | ADR-003、018 |
| Monorepo 結構與 CI/CD | ADR-019、031 |
| Run 生命週期與 Provider 契約 | ADR-004、008 |
| 安全與信任 | ADR-005、007、015；部署拓撲與安全門檻定值見 ADR-022 |
| 派送被停了、要怎麼判斷與解除 | [docs/runbooks/p1-dispatch-halt.md](docs/runbooks/p1-dispatch-halt.md)（P1 判準、誤觸分辨與解除前檢查） |
| Query 屬於哪個 context、能不能直接讀寫 | ADR-033（write）＋ADR-035（read、context 對照表對帳）＋`db/query-owners.yaml` |
| 目前還缺什麼、誰在等誰 | `docs/plans/04-backlog-and-handoffs.md`（殘項三類清單＋跨里程碑待辦） |
| 我現在要簽什麼 | [docs/plans/05-pending-rulings.md](docs/plans/05-pending-rulings.md)（逐項附事實、建議與**不決定的代價**） |
| 封測上線前要做什麼、誰做 | `docs/plans/mvp/m4/release-checklist.md`（程式面尚缺／部署期／負責人動作三段） |
| 受測者同意書與資料保存政策 | `docs/plans/mvp/gate-test/consent-and-data-policy.md`（**法務已確認 2026-08-23，§9 佔位與受測者簽署未完**；閘門與封測共用） |
| 一個畫面該長什麼樣、狀態怎麼標、停用要不要說原因 | [docs/design/system.md](docs/design/system.md)（§3 的 checklist 逐頁可用） |
| 新畫面該放哪個網址、導覽要不要加一項、入口能不能藏在旗標後面 | [docs/design/information-architecture.md](docs/design/information-architecture.md) §0（**先過這一關再寫**） |
| 語言分工與跨語言守則 | ADR-016 |
| 模型呼叫與成本 | ADR-017 |
| 評估判定、重評與 Judge 邊界 | ADR-025、026 |
| 打包下載的雜湊、簽章與可散布性 | ADR-027（＋ ADR-012、021） |
| 封測准入、免費額度強制點 | ADR-028（值由 PDM-010／PDM-009 給） |
| 漏斗量測、分析事件與 audit 的分野 | ADR-029（＋ ADR-009） |
| 目前所有未決議題 | 各 ADR 的「待決策」章節＋ `docs/plans/03` 第 1 節 |
