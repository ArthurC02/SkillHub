# ADR-033：sqlc Query Ownership 與跨 Context 寫入強制

- 狀態：Accepted
- 日期：2026-08-20
- 決策者：架構規劃
- 補充：[ADR-032](./ADR-032-ddd-bounded-context-governance-for-platform.md) 的 context 對照表與 depguard 規則
- 相關：[ADR-002](./ADR-002-domain-boundaries-and-ownership.md)（模組擁有自己的資料存取邊界）、[ADR-030](./ADR-030-portable-developer-automation-and-contract-code-generation.md)（`automation-check` 的位置）

## 背景

ADR-032 用 package 邊界＋`apps/platform/.golangci.yml` 的 depguard 把跨 context import 機械化擋住。但 `db/sqlc.yaml` 只有一個設定，`db/queries/*.sql` 的 171 條 query 全部生成到同一個 Go package（`apps/platform/internal/platform/db/gen`）。depguard 只看得到 import，看不到呼叫的是誰的 query——任何 context 只要 import 了 `db/gen`（目前 14 個 context 都有），就能直接寫別人的資料表。

Platform DDD 審視報告把這列為 P1，並指出這與 ADR-002「每個模組擁有自己的資料存取邊界」有落差。實測確認的跨 context **寫入**有 15 條，分佈在五個群組（帳號刪除 purge、ingest 寫 registry、catalog 寫 skills 旗標、搜尋索引投影、objreconcile 代掃）。跨 context **讀取**更多，其中一部分（例如 policy 讀 runs 算配額）是刻意的。

## 決策

**Query ownership 以宣告檔強制，先鎖 write，read 只宣告不擋。**

1. 新增 `db/query-owners.yaml`，為每一條 query 宣告擁有它的 context。判準是**該 query 主要資料表的擁有 context**（context 清單即 ADR-032 §1）。`artifacts` 是唯一被兩個 context 共用的表，依 `kind` 逐 query 指派：run 輸出歸 `run`，`download_package` 歸 `packaging`。
2. `go -C tools/devctl run . automation-check`（既有指令，CI 已在跑）新增檢查：解析 `db/queries/*.sql` 判定 write query，掃 `apps/platform/internal/**` 的非測試 Go 檔找呼叫點，由路徑推出呼叫端 context；**owner ≠ caller 的 write 呼叫即 FAIL**。
3. **只強制 write**（INSERT／UPDATE／DELETE，含 `WITH ... DELETE` 這種 CTE 寫入）。read 仍要宣告 owner，但不擋。理由：寫入才會破壞別人不知道的不變量；同一批把跨 context read 一起擋，會製造十幾條容忍條目，讓宣告本身失去訊號。
4. **完整性雙向檢查**：`db/queries` 有而宣告檔沒有的 query、宣告檔有而 `db/queries` 沒有的條目、已無呼叫點的容忍條目，三者都 FAIL。沒有這道檢查，宣告檔會在幾次改動內爛掉，變成看起來像防線的裝飾。
5. **Generic 套件**（ADR-032 §1 的 `audit`、`outbox`、`objreconcile`、`llmclient`、`skillpkg`、`platform/*`、`apiserver`）視同 context 參與判定：它們呼叫自己領域的 query（`outbox` 寫 `outbox_events`、`audit` 寫 `audit_events`）合法；呼叫別人的 write query 一樣要進容忍清單。「Generic」指的是沒有領域規則，不是可以繞過所有權。
6. **呼叫點的認定**：只掃 import 了 `db/gen` 的檔案，排除 `_test.go`。前者讓 generated package 自然落在掃描範圍外，也避免同名 Service 方法被誤判；後者讓 integration test 仍能自由建 fixture——測試不是 production 的資料存取路徑。
7. 宣告檔用 `section:` ／ `  key: value` 兩層的 YAML 子集，由 devctl 自解析。`tools/devctl` 目前**零第三方依賴**，這個形狀不值得引入第一個；解析器對子集外的寫法直接報錯，不靜默跳過。

## 考慮過但拒絕

**拆 sqlc 成 per-context package**（`db/sqlc.yaml` 多組設定，各 context 一個 generated package）。這是最徹底的作法——邊界會變成型別問題，depguard 直接就能擋。拒絕的理由：

- 要改動全部 14 個 context 的呼叫點與 import，且 `apps/platform/internal/platform/db/gen` 的型別（`gen.CreateRunParams` 等）散佈在 service、worker 與 integration test 中，是一次橫跨整個 platform 的機械式大改。
- 同交易跨表寫入（run 狀態機在同一個 tx 內寫 `runs`＋`run_status_transitions`＋`outbox_events`＋`audit_events`）會需要跨 package 共用 `pgx.Tx` 與多個 `Queries` handle，wiring 成本落在最敏感的一條路徑上。
- 它解決的是「能不能呼叫」，但本 repo 真正缺的是「誰該擁有」的答案。宣告檔先把答案寫下來並鎖住 write，成本是一個檔案加一段檢查；等 15 條存量漂移清完，再拆 package 才是低風險的機械操作。

**改 `db/queries/*.sql` 加 owner 註解**。`task gen` 只有主 Agent 能跑，改 .sql 有觸發 sqlc drift 的風險，且 owner 資訊會散在 18 個檔裡，看不出全貌。

**新增獨立的 Taskfile task**。`automation-check` 已在 CI 跑，多一個入口只會多一個可以忘記跑的東西。

## 存量漂移與清除路徑

導入時的 15 條跨 context write 全部列在 `db/query-owners.yaml` 的 `allow:`，標記 `# drift: DDD-015`。**`allow:` 是存量清單，不是擴充點**——新的跨 context write 一律改程式。四條清除路徑：

1. **帳號刪除 purge（6 條，`identity` → `analytics`／`testlab`／`run`／`registry`／`ingest`）**：`identity` 的 purge 事務直接清掉五個 context 的列。正解是 purge 只發 `account.deletion_due`，各 context 訂閱後自清（Outbox 已具備此能力，ADR-008）；代價是刪除從單一交易變成最終一致，需要先定義「清完」的判準。
2. **ingest 寫 registry（3 條）**：AGENTS.md 指定 `ingest` 是「版本寫入的唯一驗證路徑」，於是它直接寫 `skills`／`skill_versions`。正解是 `registry` 提供一支要求驗證憑據的 write API，`ingest` 呼叫它而非呼叫 query——驗證仍在 ingest，寫入回到 owner。
3. **搜尋索引投影（3 條，`registry`／`ingest` → `catalog`）**：`search_documents` 是 `catalog` 的投影，卻由來源資料的 owner 直接 upsert。正解是索引寫入收到 `catalog` 的 projection 服務後面，上游只發事件。
4. **旗標與代掃（3 條）**：`catalog` 寫 `skills.access_restriction`、`objreconcile` 代 `packaging`／`testlab` 更新它們的列。正解分別是 `registry` 開一支受限的 restriction 寫入 API，以及掃描器只回報差異、由 owner context 決定怎麼收。

清除順序建議照審視報告的敏感度：`skills`／`skill_versions`（路徑 2）→ `download_artifacts`（路徑 4 的 objreconcile 半邊）→ 其餘。

## 後果

### 正面

- 「哪個 context 擁有哪張表」第一次有了單一、可被 CI 驗證的答案。
- 新的跨 context 寫入在 CI 就被擋下，不必依賴 review 抓。
- 15 條既有耦合從隱形變成有名字、有清除路徑的待辦，而不是散在程式裡的慣例。
- 不動 `db/sqlc.yaml`、不動 `db/queries/*.sql`、不動任何 generated 目錄，沒有 codegen drift 風險。

### 代價

- 新增 query 要多改一個檔；漏了 CI 會 FAIL（這是刻意的）。
- read 仍是敞開的：`policy` 讀 `runs`、`analytics` 讀 `runs` 這類跨 context 讀取只被宣告、不被擋。
- 呼叫點判定是文字比對（`.<QueryName>(` ＋ `db/gen` import），不是型別解析。同名的自家方法在同一個 import 了 `db/gen` 的檔案裡會被計入；目前無此情形，若日後誤判，升級路徑是改用 `go/ast` 解析而不是放寬規則。

## 不變事項

- ADR-032 的 context 對照表仍是 context 清單的事實來源；新增 context 要同時登記在 ADR-032 §1 與 `tools/devctl/query_owners.go` 的清單。
- depguard 的跨 context import 規則不變，本檢查是它的補集不是替代。
- generated file 禁止手改、單一 Writer、以及所有實作鐵律不變。

## 待決策

- read ownership 何時開始強制，以及是否要先引入 read projection／ACL 型別（等路徑 3 完成後重新評估）。
- 存量漂移清完後是否拆 sqlc per-context package（見「考慮過但拒絕」）。
