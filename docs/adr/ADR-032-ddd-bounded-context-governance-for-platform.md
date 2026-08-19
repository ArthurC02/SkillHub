# ADR-032：Platform 的 DDD Bounded Context 治理與機械強制

- 狀態：Proposed
- 日期：2026-08-19
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
| Skill Registry & Versioning | Core | `registry`、`skillpkg`、（自 `ingest` 拆出的套件儲存面） | SKILL |
| Trust & Supply Chain | Core | `ingest`（拆分後保留的匯入管線面） | SKILL、SEC |
| Test Lab | Core | `testlab` | TEST |
| Run Orchestration | Core | `run` | RUN、SBX |
| Evaluation & Improvement | Core | `eval` | EVAL |
| Packaging & Distribution | Core | `packaging` | PACK |
| Policy & Usage | Supporting | `analytics`（quota 計數面目前寄居 `run`，拆分屬待決策） | PDM、NFR |
| —（跨切面，非 context） | Generic | `audit`、`outbox`、`objreconcile`、`llmclient`、`platform/*`、`apiserver`、`api/gen` | — |

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

## 考慮過的替代方案

- **全面戰術 DDD（每 context 鋪 aggregate／repository／domain service 三層）**：拒絕。多數 context 不變量稀薄，三層是儀式成本；Go 慣例（package 即模組、struct 即 aggregate）已覆蓋所需。
- **維持現狀（邊界靠 ADR-002 文字與註解）**：拒絕。三個月內已漂移四處，且未來寫碼者多為 Coding Agent——沒有 CI 紅線的規則對 Agent 等於不存在。
- **直接拆微服務**：拒絕。ADR-010 的拆分條件（負載、組織、安全需求）未觸發；模組化單體＋強制邊界正是為了讓未來拆分是搬運而不是手術。

## 影響

- 正面：邊界從「讀過 ADR 的人才知道」變成「CI 會擋」；Agent 快速迭代下新增程式碼有機械歸位規則；未來拆服務時 context 即拆分單位。
- 成本：depguard 白名單需隨架構演進維護；ingest 拆分與 run→eval 事件化是有風險的重構，需依調整計畫分階段執行；同步改兩處（附錄＋lint 規則）的紀律有摩擦——這個摩擦是刻意的。

## 待決策

- Policy & Usage 是否從 `run`／`analytics` 中抽成獨立套件，或等計費需求成立再抽。
- `testlab` snapshot 是否抽成獨立公開契約子包，或以文件標註公開面即可。
- 事件目錄（outbox 事件 schema）是否進 `contracts/`，或以 Go 型別＋文件為準。

## 附錄 A：跨 context import 白名單（初版＝凍結現況）

依賴方向以「A → B」表示 A import B。`platform/*`、`audit`、`outbox`、`api/gen` 對所有 context 開放（Generic），不列。

| 依賴 | 判定 | 處置 |
| --- | --- | --- |
| `apiserver` → 全部 context | 表現層／composition root，合法 | 保留 |
| `run` → `testlab`（snapshot） | 同步查詢，合法 | 保留 |
| `run` → `eval`（JobArgs 入隊） | **drift: DDD-005**，方向反轉 | 事件化後移除 |
| `eval` → `testlab`、`trace` | 同步查詢，合法 | 保留（trace 改注入，DDD-004） |
| `eval` → `ingest`（SaveVersion 等） | **drift: DDD-006**，應依 ADR-002 由 Registry 建新版本 | ingest 拆分後改依 Registry 公開 API |
| `packaging` → `ingest`、`testlab` | **drift: DDD-006**（ingest 部分） | 同上；testlab 部分為同步查詢，保留 |
| `catalog` → `ingest`、`analytics` | **drift: DDD-006**（ingest 部分）；analytics 為投影事實，合法 | ingest 部分同上 |
| `run` → `ingest`（gateb） | **drift: DDD-006** | 同上 |
| 各 context → `identity`（SessionUser／Workspace scope） | 鐵律 3 的入口，合法 | 保留 |
| 各 context → `llmclient` | ACL，合法（僅 `eval`、`catalog` 使用） | 保留 |
