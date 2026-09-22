# Platform Context Map

`apps/platform/internal/` 的 Bounded Context 對照表與跨 context import 白名單。這是活文件：新增套件、搬移路徑、開一條跨 context import，都先改這裡，再建目錄或寫 import。治理規則與理由見 [ADR 索引〈Platform Bounded Context 與 Context Map〉](../adr/README.md#platform-bounded-context-與-context-map)。

`automation-check` 用這份文件對帳：

- `context-map`：所有 Go package 目錄、下表的 `Boundary ID`／`現行 internal path`、`apps/platform/.golangci.yml` 的 depguard `files` glob，三方任一方向缺漏即 FAIL。
- `depguard-deny`：白名單與 depguard 的 deny 內容相符。
- `service-construction`：組裝套件以外，不得在方法內建構其他 context 的 `Service`。
- `query-owner`：`db/query-owners.yaml` 的 owner 必須是下表的 `Boundary ID`。

## Context 對照表

每個 Go package 目錄必須由下表一列的 `現行 internal path` 完整段落匹配。`Boundary ID` 是 query ownership、caller 回報與搬遷期間都不變的機械鍵：搬移只改 path，不改 Boundary ID。欄位順序固定；path 只能是小寫 path segment，或唯一的結尾 `/*`；重複或前綴重疊一律 FAIL。

| 產品／Bounded Context | 類型 | Boundary ID | 現行 internal path | 需求 ID 前綴 |
| --- | --- | --- | --- | --- |
| 創作者帳戶與工作區／Identity & Workspace | Core | identity | creator/workspace | WS、SEC |
| 互動 Skill 創作／Skill Creation | Core | creation | creator/creation | GEN |
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
| 創作者 Credit 帳務／Credit Ledger | Supporting | credit | creator/credit | CRED |
| — | Shared Kernel | skillpkg | shared/skillpkg | — |
| — | Generic | audit | foundation/observability/audit | — |
| — | Generic | outbox | foundation/messaging/outbox | — |
| — | Generic | objreconcile | foundation/storage/objreconcile | — |
| — | Generic | llmclient | foundation/integration/llmclient | — |
| — | Generic | modelbudget | foundation/integration/modelbudget | — |
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
| — | Generic | worker | entrypoint/worker | — |
| — | Generic | wiring | entrypoint/wiring | — |

- 產品領域名稱供人讀導覽；`類型`、`Boundary ID`、`現行 internal path` 是 CI 的 architecture identity。每個 package 只有一個 architecture identity。
- Core、Supporting、Shared Kernel 與 Generic 都要求 depguard coverage。例外是組裝套件（`apiserver`、`worker`、`wiring`，名單是 `tools/devctl` 的 `compositionRoots`）與 generated transport `entrypoint/api/gen`。
- Generic 列不得包含領域規則：`audit` 與 `outbox` 是交易內外送事件的機制；`llmclient` 與 `run` 的 provider gateway 是防腐層；`modelbudget` 只存一個秒數與是誰設的，哪些 kind 存在、值可以低到哪裡由呼叫端的 context 決定；`foundation/*`（含 generated persistence）是純技術基座；`entrypoint/*` 是表現層與組裝。

## 核心術語

各 Context 的名稱說明它負責什麼，這裡定義它經手的東西是什麼。

**Run**：一次對單一 Skill Version、針對單一凍結 Test Case 快照所請求的試跑，屬於某個 Workspace，經由一到多次 attempt 在 Sandbox Provider 上執行，最終落在單一終態。一次 attempt 才是一次實際執行；Run 是可以重試的那個單位，改派換掉的是 attempt 與它的 Provider，不是 Run 的身分。

## 跨 context import 白名單

「A → B」表示 A import B。Generic 套件（`foundation/*`、`foundation/persistence/db/gen`、`entrypoint/api/gen`）與 Shared Kernel `shared/skillpkg` 對所有 context 開放，不列。機器版是 `apps/platform/.golangci.yml` 的 depguard 規則：任何跨 context 的新 import，同一個 commit 同時改這張表與 depguard。測試檔（`_test.go`）不受規則約束。

| 依賴 | 判定 | 處置 |
| --- | --- | --- |
| `apiserver` → 全部 context | 表現層／composition root，合法 | 保留 |
| `worker` → 全部 context | composition root，合法——它按定義必須 import 每一個 context，與 `apiserver` 同一種 architecture identity | 保留 |
| 全部 context → `entrypoint/worker` | **禁止**，與 `apiserver`、`wiring` 同一條規則：沒有任何 context 可以 import 一個組裝套件 | — |
| `run` → `testlab`（snapshot 建立、dataset grant、排程讀取） | 同步查詢，合法 | 保留 |
| `run` → `trace`（寫入 Run Trace 事件） | 同步寫入，合法 | 保留 |
| `run` → `policy`（create-run 交易內問額度、讀 quota 顯示面） | Customer–Supplier，合法——「當下決策需要的事實」 | 保留 |
| `packaging` → `policy`（建立 Download Artifact 前問 retention） | Customer–Supplier，合法——沒有已核定的保存期就不建產物 | 保留 |
| `ingest` → `policy`（生成前問額度，GEN-004） | Customer–Supplier，合法——同 `run` → `policy`：規則在 policy，強制點在問問題的 context | 保留 |
| `eval` → `testlab`、`trace` | 同步查詢，合法 | 保留 |
| `eval` → `ingest`（SaveVersion 等） | Customer–Supplier，合法——採納建議必須重用匯入的完整驗證管線；第二條版本建立路徑＝第二個真相 | 保留 |
| `packaging` → `testlab` | 同步查詢，合法 | 保留 |
| `catalog` → `analytics` | 投影事實，合法 | 保留 |
| `ingest` → `registry`（匯入路徑寫入 skills／skill_versions） | Customer–Supplier，同步寫入，合法——驗證管線在 ingest，資料表的寫入回到 owner | 保留 |
| `catalog` → `registry`（下架旗標寫入 skills.access_restriction） | Customer–Supplier，同步寫入，合法——理由碼、可顯示句、operator 路由與 audit 都在 catalog，欄位寫入與取鎖回到 owner | 保留 |
| 各 context → `identity`（SessionUser／Workspace scope） | 所有使用者資料查詢的 Workspace Scope 入口，合法 | 保留 |
| `creation` → `credit`（每步結算前查餘額與門檻、結算後寫成本事件與扣點分錄） | Customer–Supplier，合法——與 `run` → `policy` 同一種「強制點在問問題的 context，規則在被問的 context」 | 保留 |
| `ingest` → `credit`（單次生成前查門檻、生成後寫成本事件與扣點分錄；匯入時的索引增強也在這裡寫成本事件） | Customer–Supplier，合法——同上，形狀對齊 `ingest` → `policy` | 保留 |
| `eval` → `credit`（評審／建議寫成本事件與扣點分錄） | Customer–Supplier，合法——同上 | 保留 |
| `catalog` → `credit`（搜尋 embedding 寫成本事件；MVP 不對搜尋扣點） | Customer–Supplier，合法——只寫入不查詢門檻，因為搜尋不受閘擋 | 保留 |

- `creation` 不 import 引用、接納與試跑的 context，這些依賴經組裝套件注入的窄介面反轉；其他 context 也不得 import `creation`。
- `credit` 不 import `identity`；它需要的 workspace 事實由兩個組裝套件各自注入一個回傳 `credit.WorkspaceFacts` 的函式。
- `objreconcile` 是 Generic 掃描器，不 import 任何 context；它需要的標記函式由組裝套件注入。
- 移出白名單的項目不得再加回；要加回，先改 ADR。
