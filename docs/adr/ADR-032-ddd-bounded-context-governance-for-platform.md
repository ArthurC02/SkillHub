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

ADR-002 的領域模組正式對映為 Bounded Context。每個 Go package dir 必須由下表一列的 `現行 internal path` 完整段落匹配；`Boundary ID` 是 query ownership、caller 回報與未來搬遷期間不變的機械鍵。新增 Context 或移動 path 必須先改本表。

| 產品／Bounded Context | 類型 | Boundary ID | 現行 internal path | 需求 ID 前綴 |
| --- | --- | --- | --- | --- |
| 創作者帳戶與工作區／Identity & Workspace | Core | identity | creator/workspace | WS、SEC |
| Skill 探索／Catalog & Discovery | Core | catalog | skill/discovery | DISC |
| Skill 資產與版本歷史／Skill Registry & Versioning | Core | registry | skill/library | SKILL |
| Skill 接納與信任／Trust & Supply Chain | Core | ingest | skill/admission | SKILL、SEC |
| 試跑情境設計／Test Lab | Core | testlab | trial/design | TEST |
| Skill 試跑執行／Run Orchestration | Core | run | trial/execution | RUN、SBX |
| 成果判定與改善／Evaluation & Improvement | Core | eval | trial/improvement | EVAL |
| Skill 交付與安裝／Packaging & Distribution | Core | packaging | skill/delivery | PACK |
| 執行證據／Run Trace | Supporting | trace | trial/evidence | TRACE |
| 創作者使用權益與資料生命週期／Policy & Usage | Supporting | policy | product/entitlements | PDM、NFR |
| 創作者旅程學習／Product Analytics | Supporting | analytics | product/learning | O11Y、PDM |
| — | Shared Kernel | skillpkg | shared/skillpkg | — |
| — | Generic | audit | foundation/observability/audit | — |
| — | Generic | outbox | foundation/messaging/outbox | — |
| — | Generic | objreconcile | foundation/storage/objreconcile | — |
| — | Generic | llmclient | foundation/integration/llmclient | — |
| — | Generic | queue | foundation/messaging/queue | — |
| — | Generic | objstore | foundation/storage/objstore | — |
| — | Generic | metrics | foundation/observability/metrics | — |
| — | Generic | partition | foundation/persistence/partition | — |
| — | Generic | pgconv | foundation/persistence/pgconv | — |
| — | Generic | envx | foundation/runtime/envx | — |
| — | Generic | httpx | foundation/runtime/httpx | — |
| — | Generic | platform | foundation/persistence/db/gen | — |
| — | Generic | apiserver | entrypoint/api/apiserver | — |
| — | Generic | api | entrypoint/api/gen | — |

2026-08-20（DDD-006）：原設想自 `ingest` 拆出的「套件儲存面」經盤點實為無狀態 zip 讀取 helper（`PackageFS`／`PackageRoot`／`MaxZipBytes`），已移入 Shared Kernel `skillpkg`；版本寫入與 Trust 驗證管線不可分（M4 PACK-002 重用裁定），留在 `ingest`。

2026-08-20（DDD-014）：Policy & Usage 完成抽離。`policy` 是新套件，收下兩條原先寄居他處的規則——PDM-010 的 quota 上限、計數查詢與拒絕判定（原 `run/quota.go`），以及 Download Artifact retention 的 fail-closed 判定（原 `packaging` 內一行 `<= 0` 檢查，債務帳 `GOV-RETENTION-001`）。**強制點沒有跟著搬**：`policy.EnforceQuota` 由 `run` 在 create-run 交易內、`requireRunSlot` 的 advisory lock 之下呼叫，ADR-028 決策 2 的時點不變；`policy` 只決策，不動作。`analytics` 維持獨立的 Supporting，**不併入** `policy`——ADR-029 的漏斗量測有自己的保存期、自己的揭露端點與自己的稽核邊界，與此處共用的只有「retention」這個詞。Workspace 併發上限（`run.MaxConcurrentRunsPerWorkspace`）留在 `run`：那是「同時能有幾個 Run」的 Run Orchestration 不變量，不是額度；`run` 在輸出畫面時把它併進 quota 的四項顯示，那是顯示決定不是所有權。未來計費以 `policy` 為家（ADR-011 的 Policy & Usage 所有權清單）。

`trace` 原稿漏列，2026-08-20 定案時補入：它擁有 Run Trace 事件的遮罩、入庫與讀取（ADR-009 的 Run Trace 平面），`run` 同步寫入、`eval` 同步讀取。

2026-08-21（[ADR-035](./ADR-035-read-ownership-enforcement-and-context-map-completeness.md) 實作註記）：**本表現在由 CI 對帳**。`devctl automation-check` 讓三份清單互相驗證——所有 Go package dir、本表的 `Boundary ID`／`現行 internal path`、`apps/platform/.golangci.yml` 的 depguard `files` glob——任一方向缺漏即 FAIL，AGENTS.md 第 11 條的「新增套件必須先在本表登記」因此第一次有強制力。2026-08-22（ADR-038 授權）：表格欄位順序固定為「產品／Bounded Context、類型、Boundary ID、現行 internal path、需求 ID 前綴」；path 只能是小寫 path segment，或唯一的 terminal `/*`，重複或 prefix overlap 一律 FAIL。這讓 flat layout 先持續通過，之後 nested path 可由完整段落最長前綴解析。

2026-08-22（[ADR-037](./ADR-037-product-analytics-and-package-architecture-identities.md)）：本表原先同時把 `skillpkg` 列入 Skill Registry 並在 §2 稱為 Shared Kernel，也把 `analytics` 與 `policy` 放在同一列。ADR-037 修訂為每個 package 只有一個 architecture identity：`skillpkg` 只屬 Shared Kernel，Product Analytics 與 Policy & Usage 各自是 Supporting Bounded Context；Core、Supporting、Shared Kernel 與 Generic 現在都要求 depguard coverage，僅 `entrypoint/api/apiserver` 與 `entrypoint/api/gen` generated transport 例外。表格現值是供 CI 解析的修訂後事實來源，原決策與修訂原因保留於 ADR-037。

Generic 列的套件**不得包含領域規則**：`foundation/observability/audit` 與 `foundation/messaging/outbox` 是鐵律 9 的機制、`foundation/integration/llmclient` 與 `run` 內的 provider gateway 是防腐層（Anticorruption Layer）、`foundation/*`（包含 `foundation/persistence/db/gen` generated persistence）是純技術基座、`entrypoint/api/apiserver` 是表現層與 composition root。

2026-08-22（[ADR-038](./ADR-038-platform-product-domain-language-and-value-stream-navigation.md)，Accepted）命名釐清：本表的產品領域名稱供人讀導覽；`類型`、`Boundary ID` 與 `現行 internal path` 是 CI 的 architecture identity 對照。人讀文件可先用「試跑情境設計」→ `testlab`、「Skill 試跑執行」→ `run`、「執行證據」→ `trace`；仍須以本表的 stable Boundary ID 決定 owner、depguard 與 query ownership。後續實體遷移只改 path，不改 stable Boundary ID。

2026-08-22（ADR-038 Phase 3、Batch 1）：`identity` 已遷移至 `creator/workspace`；stable Boundary ID、Go `package identity`、公開 API 與 query owner 均維持不變。

2026-08-22（ADR-038 Phase 3、Batch 2）：`registry` 已遷移至 `skill/library`；stable Boundary ID、Go `package registry`、公開 API 與 query owner 均維持不變。

2026-08-22（ADR-038 Phase 3、Batch 3）：`ingest` 已遷移至 `skill/admission`；stable Boundary ID、Go `package ingest`、公開 API 與 query owner 均維持不變。

2026-08-22（ADR-038 Phase 3、Batch 4）：`analytics` 已遷移至 `product/learning`；stable Boundary ID、Go `package analytics`、公開 API 與 query owner 均維持不變。

2026-08-22（ADR-038 Phase 3、Batch 5）：`catalog` 已遷移至 `skill/discovery`；stable Boundary ID、Go `package catalog`、公開 API 與 query owner 均維持不變。

2026-08-22（ADR-038 Phase 3、Batch 6）：`testlab` 已遷移至 `trial/design`；stable Boundary ID、Go `package testlab`、公開 API 與 query owner 均維持不變。

2026-08-22（ADR-038 Phase 3、Batch 7）：`policy` 已遷移至 `product/entitlements`；stable Boundary ID、Go `package policy`、公開 API 與 query owner 均維持不變。

2026-08-22（ADR-038 Phase 3、Batch 8）：`packaging` 已遷移至 `skill/delivery`；stable Boundary ID、Go `package packaging`、公開 API 與 query owner 均維持不變。

2026-08-22（ADR-038 Phase 3、Batch 9）：`trace` 已遷移至 `trial/evidence`；stable Boundary ID、Go `package trace`、公開 API 與 query owner 均維持不變。

2026-08-22（ADR-038 Phase 3、Batch 10）：`run` 已遷移至 `trial/execution`（包含 `providertest`）；stable Boundary ID、Go `package run`、公開 API 與 query owner 均維持不變。

2026-08-22（ADR-038 Phase 3、Batch 11）：`eval` 已遷移至 `trial/improvement`；stable Boundary ID、Go `package eval`、公開 API 與 query owner 均維持不變。

### 2. Context 間關係只有四種，且各有固定機制

| 關係 | 機制 | 適用 |
| --- | --- | --- |
| 同步查詢（Customer–Supplier） | import 對方套件的公開 Service API | 「當下決策需要的事實」：preflight 查 quota、run 建立時取 Test Case snapshot |
| 領域事件（Published Language） | Transactional Outbox（ADR-008），consumer 冪等 | 「觸發後續反應」：Run 終態 → 排評估、版本建立 → 重建索引投影 |
| Shared Kernel | `skillpkg`（套件格式與驗證的純函式庫） | 無狀態、無政策的共用碼；擴充需使用它的 context 共同同意 |
| 防腐層（ACL） | `llmclient`、`run` 的 provider gateway、（未來）payment 等外部系統一律經手寫轉譯層 | 外部契約不得滲入領域模型；外部型別止於 ACL |

**判準**：跨 context 的呼叫若失敗會導致「當下這筆請求無法正確回應」→ 同步；若只是「之後該發生的事沒發生」→ 事件。這回答 ADR-002 待決策第 2 項。

### 3. 邊界規則以 depguard 機械強制

`.golangci.yml` 加入 `depguard` 規則，內容即本 ADR 附錄 A 的 import 白名單。規則：

- 白名單是 context map 的 CI 表述：**任何跨 context 的新 import 必須在同一個 commit 內同時修改附錄 A 與 depguard 規則**，等於強制先過架構決策再過編譯。
- 初版白名單「凍結現況」：現存的四類漂移以明確標註（`# drift: DDD-00x`）列入白名單，阻止新增越界；存量已在 DDD-001～060 收斂後清零，凍結摘要見 [M4 DDD 邊界收斂報告](../plans/mvp/m4/report-platform-ddd-boundary-convergence-2026-08-19.md)。
- 移出白名單的項目不得再加回；加回＝新 ADR。

### 4. 戰術 DDD 刻意限縮

- **Aggregate 只用於不變量密集處**：Run 狀態機（ADR-008）、Skill Version 不可變性（ADR-003）、Evaluation append-only（ADR-026）。這三處現有實作已是實質 aggregate，補文件與測試即可，不重寫。
（2026-08-20 實作註記之三：**Skill Version 不可變性的「補文件與測試」已完成**，涵蓋三件事。

一、**不變量宣告**：`apps/platform/internal/skill/library/doc.go`。這個 context 的兩張表規則相反——`skills` 可變（summary、access_restriction、redistribution、takedown_at、deleted_at 都是寫完才填的），`skill_versions` 一寫不改。doc.go 逐條列出版本 aggregate 的五條不變量與唯一寫入路徑（`write.go` 的匯入路徑、`registry.go` 的 Fork）。

二、**機械防線補在 CI 側**：這裡要更正一個常見誤讀——不可變性**從一開始就有機械防線**，是 `db/migrations/0005_immutability.sql`／`0013` 的 `enforce_immutable()` trigger，`db/tests/immutability_test.sql` 有可跑的證明。缺的不是「有沒有擋」，而是**什麼時候知道**：trigger 要等語句真的執行才 RAISE。新增的是 `db/query-owners.yaml` 的 `immutable:` 段落與 `tools/devctl` 的對應檢查（併入既有 `automation-check`，不新增 task），任何命中這些表的 UPDATE／DELETE 在寫出來的那個 PR 上就是紅的。

三、**兩邊互相對帳**：`immutable:` 宣告的每一張表，在 `db/migrations` 必須有一個**無 WHEN、無欄位參數**的 `enforce_immutable()` trigger，否則 FAIL。因此單獨拿掉 trigger、或單獨刪掉宣告，都過不了 CI——要退場必須兩邊一起動，而那是一次被 review 看得見的動作。只列無條件凍結的八張表（`skill_versions`、`test_case_snapshots`、`run_status_transitions`、`trace_events`、`audit_events`、`download_artifacts`、`download_records`、`evaluation_model_usage`）；`runs`／`run_attempts`／`evaluations`／`evaluation_suggestions` 是列級或欄位級凍結，不是「這張表不可寫」，納入只會製造假警報。唯一豁免是帳號刪除 purge 的 `PurgeUnreferencedSkills`，形狀比照 `allow:`：具名、有理由、有清除路徑，且資料庫側走的是同一個具名旗標 `SET LOCAL skillhub.purge = 'on'`。）

- **不引入 repository interface 層**：sqlc per-context queries ＋ Go package 邊界已提供等價的封裝；在其上再鋪 interface 是單一實作的投機抽象。
- **不採 event sourcing、不引 CQRS 框架**：Catalog 的搜尋投影（`reindex`）已是夠用的手工 CQRS。
- **transaction script 是合法模式**：CRUD 密集、不變量稀薄的 context（analytics、audit 寫入）維持現狀。

### 5. Composition root 唯一化

`entrypoint/api/apiserver.NewApp(Config) (http.Handler, error)` 是 context wiring 的唯一地點；`cmd/api` 與整合測試都必須呼叫它。領域 Service 一律由 NewApp 注入，**禁止在方法內現場建構其他 context 的 Service**。

（2026-08-20 實作註記：實際簽名為 `NewApp(Config) (*App, error)`＋`App.Handler()`——整合測試需要在路由表建立前 tune `App.Deps`、建立後取得 Service 把手，回傳裸 handler 做不到這兩件事。語意不變：wiring 仍只有這一個地點。）

（2026-08-20 實作註記之二：**「唯一」的範圍是 API 這個 deployment unit，不是整個 platform**。platform 有四個 process，各自有自己的 composition root，且彼此不共用物件：

| Process | Composition root | wire 什麼 |
| --- | --- | --- |
| `cmd/api` | `entrypoint/api/apiserver.NewApp` | 全部 API context 的 Service 與 Handler；`run.Service` 只有 insert-only queue client、沒有 model gateway（鐵律 7） |
| `cmd/worker` | `cmd/worker` 的 `buildWorkers` | `run.Service`（含 gateway 與可工作的 queue client）、`eval.Service`、outbox dispatcher 與全部 River worker／periodic job |
| `cmd/maintenance` | 每個子命令各自的函式 | 該次工作用得到的單一 Service；刻意不共用，否則每個 job 都要依賴其他 job 的設定 |
| `cmd/reindex` | `main()` | phase 1 直接用 generated query，phase 2 才建 `ingest.Service` |

四個 root 是刻意的（deployment unit 不同、需要的設定也不同），此註記只是把文件講準：原文的「唯一化」約束仍然成立於各 process 內部——領域 Service 只在該 process 的 root 建構，禁止在方法內現場建構其他 context 的 Service。漂移防線改為機械化：`cmd/worker/main_test.go` 與 `internal/entrypoint/api/apiserver/app_test.go` 是不需要資料庫的 wiring smoke test，關鍵依賴漏注入即紅（worker 曾因漏設 `run.Service.Queue` 導致每個 run 都沒清理，當時沒有任何測試會紅）。`cmd/maintenance` 與 `cmd/reindex` 未補測試：兩者的 wiring 是單一 struct literal 且緊接著就被使用，漏注入會在該次命令當場失敗，smoke test 抓不到額外的失效模式。）

## 考慮過的替代方案

- **全面戰術 DDD（每 context 鋪 aggregate／repository／domain service 三層）**：拒絕。多數 context 不變量稀薄，三層是儀式成本；Go 慣例（package 即模組、struct 即 aggregate）已覆蓋所需。
- **維持現狀（邊界靠 ADR-002 文字與註解）**：拒絕。三個月內已漂移四處，且未來寫碼者多為 Coding Agent——沒有 CI 紅線的規則對 Agent 等於不存在。
- **直接拆微服務**：拒絕。ADR-010 的拆分條件（負載、組織、安全需求）未觸發；模組化單體＋強制邊界正是為了讓未來拆分是搬運而不是手術。

## 影響

- 正面：邊界從「讀過 ADR 的人才知道」變成「CI 會擋」；Agent 快速迭代下新增程式碼有機械歸位規則；未來拆服務時 context 即拆分單位。
- 成本：depguard 白名單需隨架構演進維護；ingest 拆分與 run→eval 事件化是有風險的重構，需依調整計畫分階段執行；同步改兩處（附錄＋lint 規則）的紀律有摩擦——這個摩擦是刻意的。

## 待決策（三項均已於 2026-08-19 由負責人裁定）

- ~~Policy & Usage 是否從 `run`／`analytics` 中抽成獨立套件，或等計費需求成立再抽。~~ → **裁定：完成 platform DDD 化後抽離**，列為 DDD-014。**2026-08-20 已執行**（現址 `internal/product/entitlements`，見 §1 註記與附錄 A 兩列）；完整收斂摘要見 [M4 DDD 邊界收斂報告](../plans/mvp/m4/report-platform-ddd-boundary-convergence-2026-08-19.md)。
- ~~`testlab` snapshot 是否抽成獨立公開契約子包，或以文件標註公開面即可。~~ → **裁定：不抽子包，以 UI/UX 導向重整公開契約面**（單一讀寫門面＋依使用者旅程收整 HTTP 面）；DDD-052 的 read ownership 收斂見 [ADR-035](./ADR-035-read-ownership-enforcement-and-context-map-completeness.md)，凍結摘要見 [M4 DDD 邊界收斂報告](../plans/mvp/m4/report-platform-ddd-boundary-convergence-2026-08-19.md)。執行為 DDD-007。
- ~~事件目錄（outbox 事件 schema）是否進 `contracts/`，或以 Go 型別＋文件為準。~~ → **裁定：依最佳實務落 `contracts/events/`（跟隨 Run Trace 契約前例，程式註解亦早已預告此落點）**；文件目錄先行、JSON Schema 待第一個非 Go consumer 出現再補。目錄本體：[contracts/events/domain-events.md](../../contracts/events/domain-events.md)，程式側收斂為 DDD-012。

## 附錄 A：跨 context import 白名單（初版＝凍結現況）

依賴方向以「A → B」表示 A import B。`foundation/*`、`foundation/persistence/db/gen`、`entrypoint/api/gen` 等 Generic package 與 `shared/skillpkg` Shared Kernel 對所有 context 開放，不列。**機器版**是 `apps/platform/.golangci.yml` 的 depguard 規則（DDD-002）；本附錄與機器版的 `drift:` 標記集合由 `devctl automation-check` 在 CI 比對，分岔即紅。測試檔（`_test.go`）不受規則約束——整合測試的跨 context import 由 DDD-011 收斂。

2026-08-20 依實測 import graph（Docker `go list`）修正三處：補「run → trace」列；`llmclient` 的使用者實為 catalog／eval／ingest／testlab 四者，原「僅 eval、catalog」有誤，且它屬 Generic 不逐列；`run → testlab` 實況含 snapshot 建立、grant 簽發與排程三個呼叫點。

| 依賴 | 判定 | 處置 |
| --- | --- | --- |
| `apiserver` → 全部 context | 表現層／composition root，合法 | 保留 |
| `run` → `testlab`（snapshot 建立、dataset grant、排程讀取） | 同步查詢，合法 | 保留 |
| `run` → `trace`（寫入 Run Trace 事件） | 同步寫入，合法 | 保留 |
| `run` → `policy`（create-run 交易內問額度、讀 quota 顯示面） | Customer–Supplier，合法——「當下決策需要的事實」 | 保留 |
| `packaging` → `policy`（建立 Download Artifact 前問 retention） | Customer–Supplier，合法——沒有已核定的保存期就不建產物 | 保留 |
| `ingest` → `policy`（生成前問額度，GEN-004） | Customer–Supplier，合法——同 `run` → `policy`：規則在 policy，強制點在問問題的 context | 保留 |
| `eval` → `testlab`、`trace` | 同步查詢，合法 | 保留（trace 改注入，DDD-004） |
| `eval` → `ingest`（SaveVersion 等） | Customer–Supplier，合法——採納建議必須重用匯入的完整驗證管線（M4 PACK-002 裁定；第二條版本建立路徑＝第二個真相） | 保留 |
| `packaging` → `testlab` | 同步查詢，合法 | 保留 |
| `catalog` → `analytics` | 投影事實，合法 | 保留 |
| `ingest` → `registry`（匯入路徑寫入 skills／skill_versions） | Customer–Supplier，同步寫入，合法——驗證管線在 ingest，資料表的寫入回到 owner | 保留 |
| `catalog` → `registry`（下架旗標寫入 skills.access_restriction） | Customer–Supplier，同步寫入，合法——理由碼、可顯示句、operator 路由與 audit 都在 catalog，欄位寫入回到 owner | 保留 |
| 各 context → `identity`（SessionUser／Workspace scope） | 鐵律 3 的入口，合法 | 保留 |

2026-08-20：DDD-006 完成——三個 context 對 `ingest` 的依賴隨純函式移入 `skillpkg` 而消滅並移入 deny；`eval` → `ingest` 合法化如上。

2026-08-20：DDD-005 完成，`run` → `eval` 已事件化並移入 deny。終態轉移只入隊 `run_cleanup`（同 context 內部工序），評估改由 `run.succeeded`／`run.failed` 的 outbox consumer（`internal/eval` 的 `RunEventConsumer`）觸發，故該列自白名單移除。

2026-08-20：ADR-033 清除路徑 2 完成——`ingest` 不再直接呼叫 `registry` 的 `CreateSkill`／`CreateSkillVersion`／`UpdateSkillSummary`，改呼叫 `registry` 新增的三支 import 寫入 API（`CreateSkillFromPackage`／`CreateVersionFromPackage`／`UpdateSummaryFromPackage`）。該三支收呼叫端的 `pgx.Tx`、不自開交易，因此版本列、搜尋投影（INGEST-009）與 audit 事件仍在同一個交易內（鐵律 9）。`ingest` → `registry` 因此自 deny 移入白名單（上表新增一列），`db/query-owners.yaml` 的三條對應容忍條目同批移除。

2026-08-20：ADR-033 清除路徑 4 完成，兩半分別處理。**catalog 半邊**：`catalog` 不再直接呼叫 `SetSkillAccessRestriction`，改呼叫 `registry.SetAccessRestriction`；該函式收 catalog 的 `pgx.Tx`、不自開交易，故「鎖列→寫欄位→寫 audit」仍是同一個 commit（鐵律 9）。理由碼、可顯示的說明句、兩條 operator 路由與授權檢查**全部留在 `catalog`**——這正是 `catalog/restriction.go` 檔頭原本反對「把 write endpoint 搬到 registry」的理由，該反對意見在這個分法下不成立，因為沒有第二個地方知道有哪些 code。`catalog` → `registry` 因此自 deny 移入白名單（上表新增一列）。**objreconcile 半邊不動附錄 A**：`objreconcile` 是 Generic 掃描器，若 import `packaging`／`testlab` 等於把分層倒過來，故改為由 composition root（`cmd/worker` 的 `buildWorkers`）注入 `packaging.MarkArtifactPurged`／`testlab.MarkDatasetObjectLost` 兩支函式，沒有新增任何跨 context import。三條對應的 `db/query-owners.yaml` `allow:` 條目同批移除。

2026-08-21（DDD-031 實作註記）：上一段的「鎖列→寫欄位→寫 audit 仍是同一個 commit」**結論不變，但取鎖的人換了**。當時 `SELECT … FOR UPDATE` 還留在 `catalog`，只有 `UPDATE` 回到 owner；[ADR-035](./ADR-035-read-ownership-enforcement-and-context-map-completeness.md) 把這種「鎖與它保護的不變量分屬兩個 context」判為正確性問題（B 組），DDD-031 據此把鎖搬進 `registry.SetAccessRestriction`，並讓它把 before-state 當回傳值交給 `catalog`——`catalog` 不再自己讀那一列。交易仍是 `catalog` 開的、audit event 仍由 `catalog` 在同一個 commit 內寫、理由碼與授權檢查仍在 `catalog`，鐵律 9 與上表 `catalog` → `registry` 那一列的判定和處置都不變；本註記只更正**誰取鎖**這一點。同批的 `run` → `testlab` 沒有動附錄 A：該方向本來就在上表，`run` 只是改為呼叫 owner 匯出的 `testlab.LockDraft` 而不是直接下 owner 的 query。

2026-08-23（[ADR-056](./ADR-056-the-generation-allowance-is-its-own-switch-and-it-is-off.md)）：`ingest` → `policy` 自 deny 移入白名單（上表新增一列）。M5 的生成路徑要在呼叫模型之前問額度（`02:GEN-001`「不得先花錢再說」），而額度規則屬 Policy & Usage；方向與既有的 `run` → `policy`、`packaging` → `policy` 完全相同——**policy 只決策不動作，強制點留在問問題的那個 context**。**沒有動 `db/query-owners.yaml`**：計數查的是 `skill_sources`，那本來就是 `ingest` 自己的表，`CountGeneratedSkills` 落在 `skill_import.sql` 的既有 owner 之下。
