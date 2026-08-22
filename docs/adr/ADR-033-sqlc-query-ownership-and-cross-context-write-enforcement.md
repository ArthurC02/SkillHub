# ADR-033：sqlc Query Ownership 與跨 Context 寫入強制

- 狀態：Accepted
- 日期：2026-08-20
- 決策者：架構規劃
- 補充：[ADR-032](./ADR-032-ddd-bounded-context-governance-for-platform.md) 的 context 對照表與 depguard 規則
- 相關：[ADR-002](./ADR-002-domain-boundaries-and-ownership.md)（模組擁有自己的資料存取邊界）、[ADR-030](./ADR-030-portable-developer-automation-and-contract-code-generation.md)（`automation-check` 的位置）

## 背景

ADR-032 用 package 邊界＋`apps/platform/.golangci.yml` 的 depguard 把跨 context import 機械化擋住。但 `db/sqlc.yaml` 只有一個設定，`db/queries/*.sql` 的 171 條 query 全部生成到同一個 Go package（本 ADR 撰寫時為 `apps/platform/internal/platform/db/gen`）。depguard 只看得到 import，看不到呼叫的是誰的 query——任何 context 只要 import 了 `db/gen`（目前 14 個 context 都有），就能直接寫別人的資料表。

2026-08-22（ADR-040 實體搬遷註記）：generated persistence 的現行位置為 `apps/platform/internal/foundation/persistence/db/gen`；它的單一 sqlc package、來源 `db/sqlc.yaml`、query ownership 與「generated 檔不得手改」規則均不變。前段舊路徑是本 ADR 當時的歷史描述。

Platform DDD 審視報告把這列為 P1，並指出這與 ADR-002「每個模組擁有自己的資料存取邊界」有落差。實測確認的跨 context **寫入**有 15 條，分佈在五個群組（帳號刪除 purge、ingest 寫 registry、catalog 寫 skills 旗標、搜尋索引投影、objreconcile 代掃）。跨 context **讀取**更多，其中一部分（例如 policy 讀 runs 算配額）是刻意的。

## 決策

**Query ownership 以宣告檔強制，先鎖 write，read 只宣告不擋。**

1. 新增 `db/query-owners.yaml`，為每一條 query 宣告擁有它的 context。判準是**該 query 主要資料表的擁有 context**（context 清單即 ADR-032 §1）。`artifacts` 是唯一被兩個 context 共用的表，依 `kind` 逐 query 指派：run 輸出歸 `run`，`download_package` 歸 `packaging`。
2. `go -C tools/devctl run . automation-check`（既有指令，CI 已在跑）新增檢查：解析 `db/queries/*.sql` 判定 write query，掃 `apps/platform/internal/**` 的非測試 Go 檔找呼叫點，由路徑推出呼叫端 context；**owner ≠ caller 的 write 呼叫即 FAIL**。
3. **只強制 write**（INSERT／UPDATE／DELETE，含 `WITH ... DELETE` 這種 CTE 寫入）。read 仍要宣告 owner，但不擋。理由：寫入才會破壞別人不知道的不變量；同一批把跨 context read 一起擋，會製造十幾條容忍條目，讓宣告本身失去訊號。
4. **完整性雙向檢查**：`db/queries` 有而宣告檔沒有的 query、宣告檔有而 `db/queries` 沒有的條目、已無呼叫點的容忍條目，三者都 FAIL。沒有這道檢查，宣告檔會在幾次改動內爛掉，變成看起來像防線的裝飾。
5. **Generic 套件**（ADR-032 §1 的 `foundation/*`、`platform/*`、`entrypoint/api/apiserver`、`entrypoint/api/gen`）視同 context 參與判定：它們呼叫自己領域的 query（`outbox` 寫 `outbox_events`、`audit` 寫 `audit_events`）合法；呼叫別人的 write query 一樣要進容忍清單。「Generic」指的是沒有領域規則，不是可以繞過所有權。
6. **呼叫點的認定**：只掃 import 了 `db/gen` 的檔案，排除 `_test.go`。前者讓 generated package 自然落在掃描範圍外，也避免同名 Service 方法被誤判；後者讓 integration test 仍能自由建 fixture——測試不是 production 的資料存取路徑。
7. 宣告檔用 `section:` ／ `  key: value` 兩層的 YAML 子集，由 devctl 自解析。`tools/devctl` 目前**零第三方依賴**，這個形狀不值得引入第一個；解析器對子集外的寫法直接報錯，不靜默跳過。

## 考慮過但拒絕

**拆 sqlc 成 per-context package**（`db/sqlc.yaml` 多組設定，各 context 一個 generated package）。這是最徹底的作法——邊界會變成型別問題，depguard 直接就能擋。拒絕的理由：

- 要改動全部 14 個 context 的呼叫點與 import，且單一 generated persistence package 的型別（`gen.CreateRunParams` 等）散佈在 service、worker 與 integration test 中，是一次橫跨整個 platform 的機械式大改。
- 同交易跨表寫入（run 狀態機在同一個 tx 內寫 `runs`＋`run_status_transitions`＋`outbox_events`＋`audit_events`）會需要跨 package 共用 `pgx.Tx` 與多個 `Queries` handle，wiring 成本落在最敏感的一條路徑上。
- 它解決的是「能不能呼叫」，但本 repo 真正缺的是「誰該擁有」的答案。宣告檔先把答案寫下來並鎖住 write，成本是一個檔案加一段檢查；等 15 條存量漂移清完，再拆 package 才是低風險的機械操作。

**改 `db/queries/*.sql` 加 owner 註解**。`task gen` 只有主 Agent 能跑，改 .sql 有觸發 sqlc drift 的風險，且 owner 資訊會散在 18 個檔裡，看不出全貌。

**新增獨立的 Taskfile task**。`automation-check` 已在 CI 跑，多一個入口只會多一個可以忘記跑的東西。

## 存量漂移與清除路徑

**2026-08-20 修訂**：下列第 1、3 條的補法（事件化）已由 [ADR-034](./ADR-034-cross-context-writes-close-by-inversion-not-by-events.md) 取代——改為與第 2、4 條相同的做法：寫入回到 owner、呼叫端交易原地不動，owner 的寫入函式由 composition root 注入。理由是事件化會把帳號刪除的原子性與匯入後的即時可搜尋換成最終一致，而 ADR-010 的拆分條件尚未觸發。本節其餘文字保留為當時的判斷紀錄。

導入時的 15 條跨 context write 全部列在 `db/query-owners.yaml` 的 `allow:`，標記 `# drift: DDD-015`。**`allow:` 是存量清單，不是擴充點**——新的跨 context write 一律改程式。四條清除路徑：

1. **帳號刪除 purge（6 條，`identity` → `analytics`／`testlab`／`run`／`registry`／`ingest`）**：`identity` 的 purge 事務直接清掉五個 context 的列。正解是 purge 只發 `account.deletion_due`，各 context 訂閱後自清（Outbox 已具備此能力，ADR-008）；代價是刪除從單一交易變成最終一致，需要先定義「清完」的判準。
2. **ingest 寫 registry（3 條）**：AGENTS.md 指定 `ingest` 是「版本寫入的唯一驗證路徑」，於是它直接寫 `skills`／`skill_versions`。正解是 `registry` 提供一支要求驗證憑據的 write API，`ingest` 呼叫它而非呼叫 query——驗證仍在 ingest，寫入回到 owner。

    **2026-08-20 已執行**：`registry` 新增 `CreateSkillFromPackage`／`CreateVersionFromPackage`／`UpdateSummaryFromPackage`，三者收呼叫端的 `pgx.Tx`（不自開交易，否則會拆散 INGEST-009 要求的同交易投影與 audit）並以 `skillpkg.Report` 為參數——驗證產物本身就是決定寫什麼的輸入，不另發明誰都能構造的憑據型別。`ingest` → `registry` 同批加入 ADR-032 附錄 A 與 depguard 白名單，三條 `allow:` 條目移除。
3. **搜尋索引投影（3 條，`registry`／`ingest` → `catalog`）**：`search_documents` 是 `catalog` 的投影，卻由來源資料的 owner 直接 upsert。正解是索引寫入收到 `catalog` 的 projection 服務後面，上游只發事件。
4. **旗標與代掃（3 條）**：`catalog` 寫 `skills.access_restriction`、`objreconcile` 代 `packaging`／`testlab` 更新它們的列。正解分別是 `registry` 開一支受限的 restriction 寫入 API，以及掃描器只回報差異、由 owner context 決定怎麼收。

    **2026-08-20 已執行**（兩半各自的做法不同）：

    - **旗標**：`registry` 新增 `SetAccessRestriction(ctx, tx, skillID, reason *string)`，收 `catalog` 的交易、不自開交易，因此鎖列、寫欄位與寫 audit 仍在同一個 commit（鐵律 9）。它唯一強制的規則是 0023 的 CHECK（空字串不是 code 也不是「無旗標」）；理由碼、可顯示的說明句、operator 路由、授權檢查與 audit 事件**全部留在 `catalog`**，故 `catalog/restriction.go` 檔頭原先反對「write endpoint 搬 registry ＋ export note map」的論證仍然成立而未被違反——沒有第二個地方知道有哪些 code。`catalog` → `registry` 同批加入 ADR-032 附錄 A 與 depguard 白名單。
    - **代掃**：`packaging.MarkArtifactPurged` 與 `testlab.MarkDatasetObjectLost` 由 owner context 提供，`objreconcile.Service` 以兩個 function 欄位（`RecordArtifactPurged`／`RecordDatasetLost`）由 composition root（`cmd/worker` 的 `buildWorkers`）注入。**刻意不用 import**：`objreconcile` 是 Generic，import 領域套件會把 ADR-032 的分層倒過來，故此半邊沒有新增任何跨 context import，附錄 A 不變。兩支函式收呼叫端的 `*gen.Queries`，所以 retention 半邊仍是 pool 上的單一寫入、existence 半邊仍與 audit 事件及 sighting 清除同交易。漏注入時 `Sweep` fail closed（回錯而非略過），由 `objreconcile` 與 `cmd/worker` 兩處測試守住。

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

- ~~read ownership 何時開始強制，以及是否要先引入 read projection／ACL 型別（等路徑 3 完成後重新評估）。~~ → [ADR-035](./ADR-035-read-ownership-enforcement-and-context-map-completeness.md)：**2026-08-21 起強制**，棘輪形狀與 write 相同（`read_allow:`），存量 47 組分七群、各有清除方向；不先引入 read projection／ACL 型別。同批補上「裸 SQL 的 read 盲點」的現況記錄（`eval/reconcile.go` 未隨強制一起收）。
- 存量漂移清完後是否拆 sqlc per-context package（見「考慮過但拒絕」）。

## 追加：裸 SQL 的盲點與 tripwire

**2026-08-20 已執行**：上面整套強制有一個未被強制的前提——**所有寫入都經過 sqlc**。決策 2 掃的是 `db/queries/*.sql` 的 query 呼叫點，`immutable:` 段落看的也是 query body；一行 `tx.Exec(ctx, "UPDATE skills SET ...")` 同時繞過 ownership 與 immutable 兩道檢查，且沒有任何東西會發現。導入本 ADR 時這個洞是**空的**（見下方覆核），所以補防線不必先清存量，這是成本最低的時點。

`automation-check` 因此新增第三道檢查（`tools/devctl/query_owners.go` 的「裸 SQL tripwire」）：`apps/platform/internal/**` 與 `apps/platform/cmd/**` 的非測試 Go 檔中，把含 INSERT／UPDATE／DELETE 的**字面值**交給 pgx 的 `Exec`／`Query`／`QueryRow`／`Queue` 即 FAIL，訊息指出檔案、行號與該段 SQL 開頭。生成目錄（現為 `internal/foundation/persistence/db/gen`、`internal/entrypoint/api/gen`）與 `_test.go` 不在範圍，理由同決策 6。DML 判定沿用既有的 `isWriteStatement`／`normalizeSQL`——SQL 只有一套解析。呼叫點以 `go/ast` 解析（決策後果一節提到的升級路徑，在這道檢查上先行採用），故多行 SQL 與跨行呼叫都看得到。`SendBatch` 收的是 `*pgx.Batch` 不是 SQL、`CopyFrom` 收的是識別字不是語句，故不列入入口；真正帶 SQL 的是 `Batch.Queue`。

具名豁免在 `db/query-owners.yaml` 的 `raw_sql_allow:`，形狀比照 `allow:`／`immutable_allow:`：key 是 repo 相對路徑、value 是理由（留空即 FAIL），且**失效的豁免**（該檔已無裸 DML）同樣 FAIL。**目前零條。**

### 這道檢查抓不到什麼

它是 tripwire，不是證明。以下形狀一律看不到，**不要當它完備**：

- `const q = "UPDATE ..."` 之後 `tx.Exec(ctx, q)`——SQL 不在呼叫點。`apps/platform/internal/run/halt.go` 的 `reconcilerLastRun` 就是這個形狀（它是 SELECT，不是違規，但同樣的形狀換成 DML 這裡看不到）。
- `fmt.Sprintf` 或字串串接組出來的 SQL（只看得到拼進去的字面片段）。
- pgx 以外的路徑：`database/sql`、River migration、psql。
- `apps/platform` 以外的程式。

要真正封死只能走型別——把 `pgxpool.Pool` 收在只暴露 sqlc 的 wrapper 後面，讓「拿得到 pool」本身變成不可能。那是另一個決策，成本與「拆 sqlc 成 per-context package」同級，同樣等存量漂移清完再評估。

### 導入時的覆核結果

`apps/platform/internal/**` 與 `cmd/**` 的非測試程式中，裸 SQL 呼叫共 8 處，**沒有一處是 DML**：

| 位置 | 語句 | 性質 |
| --- | --- | --- |
| `eval/eval.go:497` | `SELECT pg_advisory_xact_lock(…)` | 序列化重評的 supersede/create 配對 |
| `eval/reconcile.go:29` | 裸 `SELECT … FROM evaluations` | **已知的 read 盲點**，見下 |
| `identity/service.go:73` | `SELECT pg_advisory_xact_lock(…)` | 序列化首次登入的競態 |
| `identity/purge.go:94` | `SET LOCAL skillhub.purge = 'on'` | session 設定；`immutable_allow` 的資料庫側具名豁免 |
| `outbox/outbox.go:130`、`:139` | `SELECT pg_try_advisory_lock(…)`／`pg_advisory_unlock(…)` | publisher 單一持有者 |
| `packaging/packaging.go:580` | `SELECT pg_advisory_xact_lock(…)` | 序列化同 content hash 的打包 |
| `run/halt.go:657` | `const reconcilerLastRun`（SELECT `river_job`） | 讀 River 內部表；且 SQL 不在呼叫點，本檢查看不到 |

**`eval/reconcile.go:29` 記為已知的 read 盲點**：它直接 `Pool.Query` 讀 `evaluations`（自己 context 的表），read 本來就不強制（決策 3），所以**不是違規**；但它繞過了宣告——這條讀取不存在於 `db/query-owners.yaml`，未來 read ownership 真的開始強制時（見「待決策」），它不會自動被涵蓋，得先變成一條 query。記在這裡以免那天漏掉。
