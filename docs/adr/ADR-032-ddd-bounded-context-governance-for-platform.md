# ADR-032：Platform 的 DDD Bounded Context 治理與機械強制

- 狀態：Accepted（2026-08-20）
- 日期：2026-08-19；2026-08-20 定案——負責人授權開始執行調整計畫並委任「依最佳實務決策、詳實記錄」，三項待決策已於 2026-08-19 由負責人逐項裁定（見「待決策」節）。定案同批依實測 import graph 修正：補列 Run Trace context、修正附錄 A 三處與實況的出入（修正內容見附錄 A 註記）。
- 決策者：產品負責人（方向裁示）、架構規劃
- 關係：補充並強制 [ADR-002](./ADR-002-domain-boundaries-and-ownership.md)（不取代其領域定義）；建基於 [ADR-008](./ADR-008-asynchronous-workflows-and-domain-events.md)（Outbox 領域事件）與 [ADR-016](./ADR-016-language-and-framework-selection.md)（Go internal package）

## 背景

ADR-002 在 2026-08-13 已定義九個領域模組與所有權——那就是戰略層 DDD 的 Bounded Context。但它承諾的「開發規範需阻止跨模組直接存取」從未落成機器檢查，2026-08-19 的全 repo 架構審查證實實作已出現四類漂移：

1. **模組身分不明**：`internal/ingest` 同時是 Trust & Supply Chain 的匯入管線，又以 `SaveVersion`／`PackageFS`／`PackageRoot` 被 catalog、eval、packaging、run 四個模組當 library 用；`internal/testlab` 的 snapshot 亦同。Skill Registry context 的寫入路徑實際散在 ingest、registry、skillpkg 三處。
2. **依賴方向反轉**：`internal/run` import `internal/eval`（`run/service.go:526`）只為了把 `eval.JobArgs` 入隊——上游 context 知道下游的存在，違反 ADR-002 協作圖的方向。
3. **邊界繞道**：`eval/eval.go:375` 與 `eval/comparison.go:229` 在方法內現場 `&trace.Service{Pool: s.Pool}`，繞過 composition root，也繞過 Trace context 的完整組態（`Signer` 恆為 nil）。
4. **層次不一致**：`internal/catalog` 是唯一沒有 Service 層的模組，HTTP、pgvector 查詢與 LLM 呼叫同層；composition root 本身在 `cmd/api/main.go` 與整合測試中有兩份手抄本。

負責人已裁示：MVP 之後 platform 將持續吸收領域知識與變動，架構須以 DDD 風格治理，否則會失控。本 ADR 把 ADR-002 的邊界升級為**可由 CI 強制、可被新成員（人類或 Agent）機械遵循**的規則。

## 決策

### 1. Context 對照表（ubiquitous language 的錨點）

ADR-002 的領域模組正式對映為 Bounded Context。每個 `internal/` 套件必須屬於且僅屬於下表一列；新增套件必須先在本表登記。

| Bounded Context | 類型 | internal/ 套件 | 需求 ID 前綴 |
| --- | --- | --- | --- |
| Identity & Workspace | Core | `identity` | WS、SEC |
| Catalog & Discovery | Core | `catalog` | DISC |
| Skill Registry & Versioning | Core | `registry`、`skillpkg` | SKILL |
| Trust & Supply Chain | Core | `ingest`（匯入管線；`SaveVersion` 是版本寫入的唯一驗證路徑） | SKILL、SEC |
| Test Lab | Core | `testlab` | TEST |
| Run Orchestration | Core | `run` | RUN、SBX |
| Evaluation & Improvement | Core | `eval` | EVAL |
| Packaging & Distribution | Core | `packaging` | PACK |
| Run Trace | Supporting | `trace` | TRACE |
| Policy & Usage | Supporting | `policy`（quota 與 retention 規則）、`analytics`（漏斗量測） | PDM、NFR |
| —（跨切面，非 context） | Generic | `audit`、`outbox`、`objreconcile`、`llmclient`、`platform/*`、`apiserver`、`api/gen` | — |

2026-08-20（DDD-006）：原設想自 `ingest` 拆出的「套件儲存面」經盤點實為無狀態 zip 讀取 helper（`PackageFS`／`PackageRoot`／`MaxZipBytes`），已移入 Shared Kernel `skillpkg`；版本寫入與 Trust 驗證管線不可分（M4 PACK-002 重用裁定），留在 `ingest`。

2026-08-20（DDD-014）：Policy & Usage 完成抽離。`policy` 是新套件，收下兩條原先寄居他處的規則——PDM-010 的 quota 上限、計數查詢與拒絕判定（原 `run/quota.go`），以及 Download Artifact retention 的 fail-closed 判定（原 `packaging` 內一行 `<= 0` 檢查，債務帳 `GOV-RETENTION-001`）。**強制點沒有跟著搬**：`policy.EnforceQuota` 由 `run` 在 create-run 交易內、`requireRunSlot` 的 advisory lock 之下呼叫，ADR-028 決策 2 的時點不變；`policy` 只決策，不動作。`analytics` 維持獨立的 Supporting，**不併入** `policy`——ADR-029 的漏斗量測有自己的保存期、自己的揭露端點與自己的稽核邊界，與此處共用的只有「retention」這個詞。Workspace 併發上限（`run.MaxConcurrentRunsPerWorkspace`）留在 `run`：那是「同時能有幾個 Run」的 Run Orchestration 不變量，不是額度；`run` 在輸出畫面時把它併進 quota 的四項顯示，那是顯示決定不是所有權。未來計費以 `policy` 為家（ADR-011 的 Policy & Usage 所有權清單）。

`trace` 原稿漏列，2026-08-20 定案時補入：它擁有 Run Trace 事件的遮罩、入庫與讀取（ADR-009 的 Run Trace 平面），`run` 同步寫入、`eval` 同步讀取。

Generic 列的套件**不得包含領域規則**：`audit` 與 `outbox` 是鐵律 9 的機制、`llmclient` 與 `run` 內的 provider gateway 是防腐層（Anticorruption Layer）、`platform/*` 是純技術基座、`apiserver` 是表現層與 composition root。

### 2. Context 間關係只有四種，且各有固定機制

| 關係 | 機制 | 適用 |
| --- | --- | --- |
| 同步查詢（Customer–Supplier） | import 對方套件的公開 Service API | 「當下決策需要的事實」：preflight 查 quota、run 建立時取 Test Case snapshot |
| 領域事件（Published Language） | Transactional Outbox（ADR-008），consumer 冪等 | 「觸發後續反應」：Run 終態 → 排評估、版本建立 → 重建索引投影 |
| Shared Kernel | `skillpkg`（套件格式與驗證的純函式庫）與 `platform/*` | 無狀態、無政策的共用碼；擴充需雙方 context 同意 |
| 防腐層（ACL） | `llmclient`、`run` 的 provider gateway、（未來）payment 等外部系統一律經手寫轉譯層 | 外部契約不得滲入領域模型；外部型別止於 ACL |

**判準**：跨 context 的呼叫若失敗會導致「當下這筆請求無法正確回應」→ 同步；若只是「之後該發生的事沒發生」→ 事件。這回答 ADR-002 待決策第 2 項。

### 3. 邊界規則以 depguard 機械強制

`.golangci.yml` 加入 `depguard` 規則，內容即本 ADR 附錄 A 的 import 白名單。規則：

- 白名單是 context map 的 CI 表述：**任何跨 context 的新 import 必須在同一個 commit 內同時修改附錄 A 與 depguard 規則**，等於強制先過架構決策再過編譯。
- 初版白名單「凍結現況」：現存的四類漂移以明確標註（`# drift: DDD-00x`）列入白名單，阻止新增越界，存量由 [調整計畫](../plans/platform-ddd-realignment-2026-08-19.md) 逐項清除後移出白名單。
- 移出白名單的項目不得再加回；加回＝新 ADR。

### 4. 戰術 DDD 刻意限縮

- **Aggregate 只用於不變量密集處**：Run 狀態機（ADR-008）、Skill Version 不可變性（ADR-003）、Evaluation append-only（ADR-026）。這三處現有實作已是實質 aggregate，補文件與測試即可，不重寫。
- **不引入 repository interface 層**：sqlc per-context queries ＋ Go package 邊界已提供等價的封裝；在其上再鋪 interface 是單一實作的投機抽象。
- **不採 event sourcing、不引 CQRS 框架**：Catalog 的搜尋投影（`reindex`）已是夠用的手工 CQRS。
- **transaction script 是合法模式**：CRUD 密集、不變量稀薄的 context（analytics、audit 寫入）維持現狀。

### 5. Composition root 唯一化

`apiserver.NewApp(Config) (http.Handler, error)` 是 context wiring 的唯一地點；`cmd/api` 與整合測試都必須呼叫它。領域 Service 一律由 NewApp 注入，**禁止在方法內現場建構其他 context 的 Service**。

（2026-08-20 實作註記：實際簽名為 `NewApp(Config) (*App, error)`＋`App.Handler()`——整合測試需要在路由表建立前 tune `App.Deps`、建立後取得 Service 把手，回傳裸 handler 做不到這兩件事。語意不變：wiring 仍只有這一個地點。）

## 考慮過的替代方案

- **全面戰術 DDD（每 context 鋪 aggregate／repository／domain service 三層）**：拒絕。多數 context 不變量稀薄，三層是儀式成本；Go 慣例（package 即模組、struct 即 aggregate）已覆蓋所需。
- **維持現狀（邊界靠 ADR-002 文字與註解）**：拒絕。三個月內已漂移四處，且未來寫碼者多為 Coding Agent——沒有 CI 紅線的規則對 Agent 等於不存在。
- **直接拆微服務**：拒絕。ADR-010 的拆分條件（負載、組織、安全需求）未觸發；模組化單體＋強制邊界正是為了讓未來拆分是搬運而不是手術。

## 影響

- 正面：邊界從「讀過 ADR 的人才知道」變成「CI 會擋」；Agent 快速迭代下新增程式碼有機械歸位規則；未來拆服務時 context 即拆分單位。
- 成本：depguard 白名單需隨架構演進維護；ingest 拆分與 run→eval 事件化是有風險的重構，需依調整計畫分階段執行；同步改兩處（附錄＋lint 規則）的紀律有摩擦——這個摩擦是刻意的。

## 待決策（三項均已於 2026-08-19 由負責人裁定）

- ~~Policy & Usage 是否從 `run`／`analytics` 中抽成獨立套件，或等計費需求成立再抽。~~ → **裁定：完成 platform DDD 化（調整計畫 Phase 0～2 收斂）之後抽離**，列為 [DDD-014](../plans/platform-ddd-realignment-2026-08-19.md)。**2026-08-20 已執行**（`internal/policy`，見 §1 註記與附錄 A 兩列）。
- ~~`testlab` snapshot 是否抽成獨立公開契約子包，或以文件標註公開面即可。~~ → **裁定：不抽子包，以 UI/UX 導向重整公開契約面**（單一讀寫門面＋依使用者旅程收整 HTTP 面），設計見 [testlab-contract-design-2026-08-19.md](../plans/testlab-contract-design-2026-08-19.md)，執行為 DDD-007。
- ~~事件目錄（outbox 事件 schema）是否進 `contracts/`，或以 Go 型別＋文件為準。~~ → **裁定：依最佳實務落 `contracts/events/`（跟隨 Run Trace 契約前例，程式註解亦早已預告此落點）**；文件目錄先行、JSON Schema 待第一個非 Go consumer 出現再補。目錄本體：[contracts/events/domain-events.md](../../contracts/events/domain-events.md)，程式側收斂為 DDD-012。

## 附錄 A：跨 context import 白名單（初版＝凍結現況）

依賴方向以「A → B」表示 A import B。`platform/*`、`audit`、`outbox`、`api/gen`、`llmclient`（ACL）、`skillpkg`（Shared Kernel）對所有 context 開放（Generic），不列。**機器版**是 `apps/platform/.golangci.yml` 的 depguard 規則（DDD-002）；本附錄與機器版的 `drift:` 標記集合由 `devctl automation-check` 在 CI 比對，分岔即紅。測試檔（`_test.go`）不受規則約束——整合測試的跨 context import 由 DDD-011 收斂。

2026-08-20 依實測 import graph（Docker `go list`）修正三處：補「run → trace」列；`llmclient` 的使用者實為 catalog／eval／ingest／testlab 四者，原「僅 eval、catalog」有誤，且它屬 Generic 不逐列；`run → testlab` 實況含 snapshot 建立、grant 簽發與排程三個呼叫點。

| 依賴 | 判定 | 處置 |
| --- | --- | --- |
| `apiserver` → 全部 context | 表現層／composition root，合法 | 保留 |
| `run` → `testlab`（snapshot 建立、dataset grant、排程讀取） | 同步查詢，合法 | 保留 |
| `run` → `trace`（寫入 Run Trace 事件） | 同步寫入，合法 | 保留 |
| `run` → `policy`（create-run 交易內問額度、讀 quota 顯示面） | Customer–Supplier，合法——「當下決策需要的事實」 | 保留 |
| `packaging` → `policy`（建立 Download Artifact 前問 retention） | Customer–Supplier，合法——沒有已核定的保存期就不建產物 | 保留 |
| `eval` → `testlab`、`trace` | 同步查詢，合法 | 保留（trace 改注入，DDD-004） |
| `eval` → `ingest`（SaveVersion 等） | Customer–Supplier，合法——採納建議必須重用匯入的完整驗證管線（M4 PACK-002 裁定；第二條版本建立路徑＝第二個真相） | 保留 |
| `packaging` → `testlab` | 同步查詢，合法 | 保留 |
| `catalog` → `analytics` | 投影事實，合法 | 保留 |
| 各 context → `identity`（SessionUser／Workspace scope） | 鐵律 3 的入口，合法 | 保留 |

2026-08-20：DDD-006 完成——三個 context 對 `ingest` 的依賴隨純函式移入 `skillpkg` 而消滅並移入 deny；`eval` → `ingest` 合法化如上。

2026-08-20：DDD-005 完成，`run` → `eval` 已事件化並移入 deny。終態轉移只入隊 `run_cleanup`（同 context 內部工序），評估改由 `run.succeeded`／`run.failed` 的 outbox consumer（`internal/eval` 的 `RunEventConsumer`）觸發，故該列自白名單移除。
