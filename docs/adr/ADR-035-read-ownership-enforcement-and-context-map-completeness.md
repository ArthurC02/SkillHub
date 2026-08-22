# ADR-035：Read Ownership 開始強制，Context 對照表補上完整性檢查

- 狀態：Accepted
- 日期：2026-08-21
- 決策者：架構規劃
- 回答：[ADR-033](./ADR-033-sqlc-query-ownership-and-cross-context-write-enforcement.md) 待決策第 1 項（read ownership 何時開始強制）、[ADR-034](./ADR-034-cross-context-writes-close-by-inversion-not-by-events.md) 待決策第 2 項
- 補充：[ADR-032](./ADR-032-ddd-bounded-context-governance-for-platform.md) §1（context 對照表）、§2（關係只有四種）、§3（depguard 強制）
- 修訂：package identity 與 coverage 判定由 [ADR-037](./ADR-037-product-analytics-and-package-architecture-identities.md) 修訂

## 背景

ADR-033 為 `db/queries/*.sql` 的全部 query 宣告了 owner context，但**只強制 write**。當時的理由寫得很明白：「同一批把跨 context read 一起擋，會製造十幾條容忍條目，讓宣告本身失去訊號」。那是針對 write 存量還在（15 條 `allow:`）的當下說的——訊號會被兩種雜訊同時稀釋。

ADR-034 清完最後 6 條後，`allow:` 是 0 條。稀釋來源消失了，read 的存量因此可以有自己乾淨的一段。ADR-034 的待決策也明說了這件事：write 側存量清完後，read 是否強制「可以在乾淨的基礎上重新評估」。

另一個洞在旁邊：AGENTS.md 第 11 條寫著「新增套件必須先在 ADR-032 §1 對照表登記」，**但沒有任何東西強制它**。漏登記的套件會安靜地活在 `internal/` 底下——既不屬於任何 context，也不被 depguard 的任何一條規則涵蓋。當時現況是一致的（19 個目錄對得上 §1 的 17 列加 `platform/*`、`api/gen`），所以這是補檢查最便宜的時點，不必先清存量。

2026-08-22（ADR-040 實體搬遷註記）：現行 checker 對帳 ADR-032 的完整 nested path；generated persistence 為 `foundation/persistence/db/gen`，API composition 與 generated transport 為 `entrypoint/api/{apiserver,gen}`。本段中的數量與舊路徑是本 ADR 撰寫時的歷史量測。

## 決策

### 1. read ownership 開始強制，棘輪形狀與 write 完全相同

ADR-032 §2 規定跨 context 取事實的合法機制是**import 對方套件的公開 Service API**。直接呼叫對方的 query 不在那四種關係裡的任何一種——它是 write 漂移的 read 版，只是後果不同：讀不會破壞別人的不變量，但會凍結別人的 schema——owner 一改欄位，不知情的呼叫端就壞了，而且沒有任何地方記載誰在讀。

因此：

- `db/query-owners.yaml` 新增 `read_allow:` 段落，形狀比照 `allow:`／`immutable_allow:`／`raw_sql_allow:`——**具名、有理由、失效即 FAIL**。key 是 query 名，value 是被容忍的呼叫端 context 清單，理由按成因分組寫在各組上方的註解（與清空前的 `allow:` 同一種寫法）。
- 新的跨 context read 一律 FAIL。已無呼叫點的條目一樣 FAIL。
- **條目放錯段落也 FAIL**，訊息直接指出該搬去哪一段（`read_allow.X is a write query; declare it in allow:`，反之亦然），而且**不會給予豁免**——放錯段落的條目不擋任何東西。

### 2. 判定式是唯一的差別，解析與掃描完全重用

read 與 write 走同一個迴圈，只有 `queries[name].write` 的比對值不同。`sqlQuery` 解析、`isWriteStatement`／`normalizeSQL`、以及「掃 import 了 `db/gen` 的非測試 Go 檔、由路徑推呼叫端 context」的呼叫點掃描全部不動。沒有第二套 parser，也沒有第二個掃描器。

**Generic 套件沿用 write 側的既有規則**（ADR-033 決策 5）：`foundation/*`、`platform/*`、`entrypoint/api/apiserver`、`entrypoint/api/gen` 視同 context 參與判定——讀自己領域的 query 合法，讀別人的一樣要進 `read_allow:`。不為 read 另立一套語意。

### 3. Context 對照表完整性：三份清單互相對帳

`automation-check` 新增 `contextMapProblems`，讓三者互相對得上：

| 清單 | 事實來源 |
| --- | --- |
| `apps/platform/internal/` 底下的套件目錄 | 檔案系統 |
| ADR-032 §1 對照表「internal/ 套件」欄 | ADR-032（context 清單的事實來源） |
| `apps/platform/.golangci.yml` 的 depguard 規則 | ADR-032 §3 的機器版 |

任一方向缺漏都 FAIL，訊息指出是哪個套件、缺在哪一側：

- 目錄存在但 §1 沒登記 → `apps/platform/internal/X is not listed in ADR-032 … §1; register it before adding the package (AGENTS.md 第 11 條)`
- §1 有登記但目錄不存在 → `§1 lists "X" but apps/platform/internal/X does not exist`
- §1 的**非 Generic** 列沒有 depguard 規則 → `§1 puts "X" in a bounded context but apps/platform/.golangci.yml has no depguard rule covering it`
- depguard 規則指著不存在的套件 → `… guards apps/platform/internal/X but that package does not exist`

**depguard 覆蓋只對非 Generic 的列強制。** §1 的 Generic 列裡，`entrypoint/api/apiserver` 是 composition root（它按設計 import 每一個 context，一條 deny 規則什麼都不能寫）、`entrypoint/api/gen` 是生成碼；兩者刻意沒有規則，強制它們有規則只會逼出一條空規則。其餘 Generic 套件（`foundation/*`／`platform/*`）實際上被 `.golangci.yml` 的 `generic` 那條規則涵蓋——檢查不反對，它只要求「領域 context 一定要有人管」，不禁止 Generic 也被管。

**§1 表格的解析要處理得了實際寫法，處理不了就報錯**：儲存格會夾全形括號的註解，而註解裡也有反引號（`` `ingest`（匯入管線；`SaveVersion` 是版本寫入的唯一驗證路徑） `` 裡的 `SaveVersion` 不是套件），所以先整段移除 `（…）` 再抽反引號 token；path 以完整 segment 比對（`platform/*` 可為 terminal wildcard）；不符 nested path 規則的 token **報錯而不是靜默跳過**——安靜跳過的解析器等於沒有這道檢查。表格範圍以 `### 1. Context 對照表` 到下一個 `### ` 標題為界，因為 ADR-032 有四張表。

`skillpkg` 同時出現在 §1 的 Skill Registry 列（Core）與 §2 的 Shared Kernel／附錄 A 的 Generic 語意裡。這道檢查對此不表態：**同一個套件出現在多列時，只要有一列非 Generic 就照非 Generic 要求**，於是 `skillpkg` 需要覆蓋，而它已被 `generic` 規則覆蓋，兩種讀法都通過。要不要把 §1 講一致是另一個決定，不由這道檢查代答。

2026-08-22：[ADR-037](./ADR-037-product-analytics-and-package-architecture-identities.md) 已回答上一段刻意未答的問題；現行 parser 採唯一 `packageIdentity`，重複 package、未知 kind 與缺少 coverage 一律 FAIL。上一段保留為 ADR-035 當時行為的歷史紀錄。

### 4. 不新增 Taskfile task

兩道檢查都併入既有的 `automation-check`（CI 已在跑）。理由同 ADR-033：多一個入口只會多一個可以忘記跑的東西。`tools/devctl` 維持**零第三方依賴**。

## 2026-08-22 產品命名釐清（ADR-038 Proposed）

本 ADR 的 Context、caller 與 owner 是 ADR-032 §1 的 stable Boundary ID／package slug，因為 read ownership、depguard 與 query ownership 必須有機器可判定的鍵。它們不是產品領域名稱，也不因人讀導覽的語彙而改寫。 [ADR-038](./ADR-038-platform-product-domain-language-and-value-stream-navigation.md) 提出的「創作者空間」、「Skill 生命週期」、「試跑與改善」與「產品營運」價值流，以及其下的候選產品領域名稱，只用於產品導覽；ADR-038 未被 Accepted 前，不能據此更動本 ADR 的 owner 判定、allowlist、package path 或 Context Map parser。

## 已知盲點

- **裸 SQL tripwire 不是完整 dataflow。** DDD-058 後，pgx `Exec`／`Query`／`QueryRow`／`Queue` 的 SELECT／DML／DDL／SET（含 WITH）都會被掃描，並解析 direct literal、package const、function-local binding 與 `fmt.Sprintf` 的 format；DDD-057 也已把 Eval recovery 的領域 read 移入 sqlc。但動態拼接、跨函數傳遞或非 pgx 入口仍可能看不到。要完全封死仍只能走型別（把 Pool 收在只暴露 sqlc 的 wrapper 後面）或拆 sqlc per-context package，成本同級，維持待決策。
- **呼叫點判定仍是文字比對**（`.<QueryName>(` ＋ `db/gen` import），不是型別解析。read 側繼承 write 側的這個限制，升級路徑不變（改用 `go/ast`，裸 SQL tripwire 已先行採用）。
- **`read_allow:` 的「理由」由分組註解承載，不由機器檢查。** 與 `allow:`／`immutable_allow:` 相同（只有 `raw_sql_allow:` 的 value 就是理由，因為它的 key 是精確 `repo/path.go@FunctionName`、沒有第二個欄位可放）。機器強制的是「具名」與「失效即 FAIL」；理由的品質靠 review。

## 存量：35 個 `read_allow:` 條目、七組（DDD-031 後 33 個；DDD-033 後 26 個；DDD-035 後 23 個；DDD-036 後 22 個；DDD-037 後 20 個；DDD-038 後 19 個；DDD-039 後 17 個；DDD-040 後 16 個；DDD-041 後 14 個；DDD-042 後 11 個；DDD-043 後 9 個、一組）

導入時量到 47 組 (query, caller) 跨 context read、共 56 個呼叫點，收斂為 `read_allow:` 的 35 個條目（同一 query 的多個呼叫端合成一行）。

**本節有兩個單位，別把它們相加**：節標題的數字是 **yaml 條目數**（一個 query 一行），而下表與 `db/query-owners.yaml` 分組標題裡的「條」是 **(query, caller) 組數**——所以下表六組的「條」相加是 45（導入時七組相加是 47），不是 33（導入時 35）。

分組與清除方向如下——**逐組的判讀寫在 `db/query-owners.yaml` 的分組註解裡，本節不重寫成另一套說法**。

2026-08-21（DDD-031 實作註記）：**B 組已收，存量現為 33 個條目、六組**（45 組 (query, caller)、54 個呼叫點）。組別字母不重編，A 之後直接跳 C——下表的字母與 yaml 的分組標題永遠指同一組東西，重編會讓兩邊的歷史對不上。導入時的數字保留在上面一段，那是量測當下的事實。

2026-08-21（DDD-033 實作註記）：**C 組與 G 組已收，存量現為 26 個條目、四組**（37 組 (query, caller)、45 個呼叫點）。同樣不重編字母——現在留下的是 A、D、E、F 四組，中間的空缺是刻意的。

2026-08-22（DDD-035 實作註記）：**E 組已收，存量現為 23 個條目、三組**（34 組 (query, caller)）。packaging 與 testlab 各自以三欄 owner DTO 公開 reconcile candidates；`cmd/worker` 只做 DTO 轉換並注入三個窄 lister。Generic 的 objreconcile 不再決定 owner 哪些列該被掃，也不再直接呼叫三條 owner query；三個 read 與兩個 write dependency 任一缺漏都 fail closed。

2026-08-22（DDD-036 實作註記）：**F 組的 `ListPendingEnrichment` 已收，存量現為 22 個條目、三組**（33 組 (query, caller)）。catalog 公開四欄 owner DTO 與 lister，`cmd/reindex` 做一次 DTO 轉換後注入 ingest；`ReindexPending` 缺 read 或 write 任一 dependency 都在工作開始前 fail closed。沒有新增 repository、SQL 或 generated 變動。

2026-08-22（DDD-037 實作註記）：**F 組的兩條 packaging manifest read 已收，存量現為 20 個條目、三組**（31 組 (query, caller)）。eval 與 ingest 分別公開三欄 suggestion、五欄 source owner DTO；`NewApp` 轉換後注入 packaging。manifest 缺任一 reader 即在組裝 provenance 前 fail closed；原有 suggestion Workspace scope 與跨 Workspace lineage walk 不變。

2026-08-22（DDD-038 實作註記）：**F 組的 `CountArtifactsSharingObject` 已收，存量現為 19 個條目、三組**（30 組 (query, caller)）。Packaging Service 以自己的 Pool 回答 live artifact reference count，API／worker composition roots 將 bound method 注入 Run。缺 counter 時 Run 拒絕刪除；有 wiring 時原有 soft-delete commit → count → object remove 順序不變。

2026-08-22（DDD-039 實作註記）：**F 組最後兩項已收，存量現為 17 個條目、兩組**（28 組 (query, caller)）。policy 的 quota 計算只收 identity 建立時間 scalar 與 run usage DTO callbacks，不再 import `db/gen`。建立 Run 時，run 以同一個 transaction-backed `*gen.Queries` 組兩支 owner read，且仍在 `requireRunSlot` 取鎖後計數；`QuotaFor` 則以 pool-backed run query 與注入的 Identity Service method 讀取。F 組至此歸零。

2026-08-22（DDD-040 實作註記）：**D 組的 analytics membership read 已收，存量現為 16 個條目、兩組**（27 組 (query, caller)）。Run Service 以 `BelongsToWorkspace` 回答 bool，`NewApp` 將 bound method 注入 Analytics。foreign／missing／query error 仍只丟棄 optional `run_id` 並保存 feedback；唯獨 callback 未 wiring 會在寫入前 fail closed，因為那是 composition failure 而不是資料查詢結果。

2026-08-22（DDD-041 實作註記）：**D 組的三項 Trace read 已收，存量現為 14 個條目、兩組**（24 組 (query, caller)）。Run Service 提供 workspace-scoped state、signed-grant scope 與 ordered transitions 三個 owner DTO/read methods；API／worker composition roots 轉成 Trace 的窄 callbacks。Trace 不再 import generated Run row／query，缺任一 callback 都 fail closed；找不到仍映射 `trace.ErrNotFound`，ingest 的 workspace 仍只由 signed token 所指 Run 解出。

2026-08-22（DDD-042 實作註記）：**D 組最後三項 Eval read 已收，存量現為 11 個條目、一組**（21 組 (query, caller)）。Run Service 聚合 workspace-scoped Run facts、latest attempt（無 attempt 仍為 1）與 live output artifact evidence；Eval 只收 consumer-owned DTO 的兩支 callbacks。comparison／apply 的單筆 facts 與 gather 的聚合 input 都不再接 generated Run／Artifact row，terminal 判定改為 Eval 自己的 string 語意；缺 callback 即 fail closed。D 組至此歸零。

2026-08-22（DDD-043 實作註記）：**A 組 ingest 的三組 transactional reads 已收，存量現為 9 個條目、一組**（18 組 (query, caller)）。Registry 先開本批所需的最小 read face：三支函式收 `pgx.Tx`、回 owner DTO 與 found bool，不收／回 generated query 或 row。Ingest 仍在原 transaction 內查 Skill 與 duplicate version，故未提交列的 visibility、workspace scope、missing Skill 與 identical-content duplicate 語意不變。

2026-08-22（DDD-044 實作註記）：**A 組 Catalog 的五組 owner reads 已收，存量現為 7 個條目、一組**（13 組 (query, caller)）。Catalog 以單一 Registry Service 讀公開／私有 Skill、最新 Version 與 runtime compatibility；source provenance 改由 Ingest 的 workspace-scoped owner read 在 API composition root 轉成 Catalog callback。跨界只傳 owner／consumer DTO，anonymous catalog、private own／foreign、takedown、no-version、runtime absent 與 source availability 語意不變；`GetSkillSource` 的 query owner 同步更正為 ingest。

2026-08-22（DDD-045 實作註記）：**A 組 Run／TestLab 的三組 reads 已收，存量仍為 7 個條目、一組、但 caller pair 降為 10 組**。Registry 補 workspace-scoped Version-by-ID owner read；Run 與 TestLab 只定義各自需要的 Skill／Version facts callbacks，由 API／worker roots 轉接同一 Registry Service。workspace／skill mismatch、licensing hold、package refs、schedule grants，以及 SuggestCriteria 的 missing-skill 顯示語意不變；缺 callback 在 DB／物件／LLM 工作前 fail closed。

2026-08-22（DDD-046 實作註記）：**A 組 Eval 的四組 reads 已收，存量現為 6 個條目、一組**（6 組 (query, caller)，全為 Packaging）。Eval 以 consumer-owned Skill／Version／RuntimeCompatibility facts 接四支窄 callbacks，API／worker roots 轉接共用 Registry Service；gather、comparison、suggestion apply 不再接 generated Registry row。material、compatibility absent／present、origin／latest apply、licensing hold、non-terminal 與 comparison 語意不變，缺所需 callback 在工作前 fail closed。

2026-08-22（DDD-047 實作註記）：**A 組 Packaging 的最後六組 reads 已收，`read_allow:` 歸零**。Registry 以最小 owner DTO/API 回答 scoped Skill／Version／compatibility／previous version，以及刻意跨 Workspace 的 lineage／oldest version；Packaging 只接 consumer facts callbacks。Ingest 的 source lineage 同步改為 pool-backed Service method，跨界簽名不再帶 `*gen.Queries`。gate、skill-version 配對、compatibility absent／present、improvement previous version、fork multi-hop／root source、unavailable／cycle／32-hop 上限與 license／redistribution policy 均不變；缺 callback 在讀 DB 或物件前 fail closed。

2026-08-22（DDD-048 實作註記）：Registry 的 import write face 與 restriction write face 不再把 generated row 當跨 context contract。`CreateSkillFromPackage`／`CreateVersionFromPackage` 保留 caller transaction、改回 Registry 既有 Skill／Version DTO；`SetAccessRestriction` 回最小 `RestrictionBefore`。Ingest 刪除轉回 generated row 的 helpers，Result／HTTP／audit 直接使用 owner DTO；Catalog audit 只讀 before-state DTO。duplicate、JSON、audit metadata、row lock 與同 transaction invariant 不變。

2026-08-22（DDD-049 實作註記）：Catalog projection write face 改收 caller 的 `pgx.Tx` 與 Catalog-owned DTO，`gen.New(tx)` 留在 owner 內；Registry／Ingest 各自只把 consumer DTO 交給 composition-root adapter。匯入、fork、soft delete 與 takedown 的 projection 寫入仍與領域寫入同一 transaction。Pending enrichment worklist 改為 pool-backed `Catalog.Service` read，Ingest／reindex 不再跨界傳 DB handle；缺 read／write callback 仍在工作前 fail closed。

2026-08-22（DDD-050 實作註記）：最後一個 provenance DB carrier 已移除。`Eval.Service.AppliedSuggestions` 以自身 Pool 回 applied-only、Workspace-scoped owner DTO；Packaging callback、manifest 組裝與 API root adapter 不再傳 `*gen.Queries`。Improvement 判斷仍先於 fork/import，suggestion 順序與 manifest 欄位不變；callback 未 wiring 仍在 provenance 查詢前 fail closed。

2026-08-22（DDD-051 實作註記）：Account purge 的 DB carrier 分成正確的兩種生命週期。交易前 object-key listers 改為 TestLab／Run／Packaging Service 的 pool-backed methods，只收 `ctx/workspaceID`；交易內六個 purge callbacks 只收 caller `pgx.Tx`，各 owner 內才建 sqlc query。物件先刪、單交易六 context rows、registry-before-ingest、identity 去識別化與 audit 順序不變，API／maintenance 均注入 bound methods且缺一即 fail closed。

2026-08-22（DDD-052 實作註記）：Test Lab 的跨 context published face 不再暴露 generated row／`*gen.Queries`。Run／Eval／Packaging 共用 composition root 注入的 `*testlab.Service`；Pool reads 回 Test Lab DTO，`LockDraft`／`CreateSnapshot` 只收 caller `pgx.Tx`。Run 將已鎖定的完整 Draft 沿同一交易交給 permission summary 與 snapshot，避免為了 owner face 改回 Pool 重讀而破壞鎖與未提交可見性；Workspace scope、快照不可變、dataset hash／refs 與 package 欄位不變，缺 Service／Pool／transaction 皆 fail closed。

2026-08-22（DDD-053 實作註記）：Generic Objreconcile 的 owner transport 不再攜帶 `*gen.Queries`。Packaging／TestLab 的 candidate lists 改為 pool-backed Service methods；row correction callbacks 只接 Objreconcile 建立的 `pgx.Tx`，owner 內才 `gen.New(tx)`。過期物件仍先做冪等刪除再於交易中標記；缺件仍需連續兩次觀察，mark、audit 與 sighting cleanup 保持同一 transaction。Worker 注入共用 owner Service bound methods，缺 callback／Pool 仍 fail closed。

2026-08-22（DDD-054 實作註記）：Outbox 的跨 context contract 改為 owner `Event`／`NewEvent` DTO；Dispatcher、Worker delivery 與 producers 不再暴露 generated event row／insert params。Run status／cleanup mapping 只收 string，transactional `Insert` 仍強制 caller `pgx.Tx`。Eval consumer 以 `HasCurrentEvaluation(workspaceID,runID)` owner callback 保留 redelivery no-op，不再接 generated query params／Evaluation row；Worker 注入 bound Eval method。Claim、retry、dead-letter、payload 與 current-evaluation idempotency 語意不變，缺 consumer dependency fail closed。

2026-08-22（DDD-055 實作註記）：`audit.Log` 改收 Audit 本地最小 `DBTX`，generated Queries／Params 留在 Audit 內；不新增 Service 或 repository。所有領域寫入 caller 直接傳原 `pgx.Tx`，因此 audit 與領域狀態仍同交易；operator roster 與 provider mismatch 等原本獨立 append 的路徑直接傳 Pool，維持單 statement 語意。缺 DB handle fail closed。

2026-08-22（DDD-056 實作註記）：Identity 對其他 context 發布自身的 `User`／`Workspace` DTO，generated persistence rows 只留在 Identity 內並由集中 mapper 轉換。Session、帳號刪除申請／取消、Personal Workspace 與各領域 handler／service 不再以 `gen.User`／`gen.Workspace` 作跨界契約；既有 JSON、session 驗證、Workspace scope、owner role、刪除時間與 audit／transaction 語意不變。

2026-08-22（DDD-057 實作註記）：Evaluation recovery 的 stale-pending read 從 `reconcile.go` 內嵌 SQL 移入 `db/queries/evaluation.sql`，由 Eval owner 的 sqlc query 執行。Recovery policy 仍由 Go 傳入 cutoff 與 100 筆上限；oldest-first、pending-only、未 supersede 與逐筆 recovery 語意不變，並納入 query ownership 與 codegen drift 檢查。

2026-08-22（DDD-058 實作註記）：裸 SQL tripwire 從 direct-literal DML 擴為函數級 SQL 掃描：辨識 SELECT／INSERT／UPDATE／DELETE／SET／CREATE／ALTER／DROP／TRUNCATE 與 WITH，解析 direct literal、package const、function-local binding、`fmt.Sprintf` format。`raw_sql_allow:` 改以 `repo/path.go@FunctionName` 精確豁免且 stale 必敗；九個既有技術 SQL 函數逐一附理由，其他同檔函數不受遮蔽。generated 與 test exclusions 不變。

2026-08-22（DDD-059 實作註記）：Trace 最後兩個跨 context generated carrier 已移除。`RecordOrchestratorEvent` 只收 caller `pgx.Tx`，owner 內才建 sqlc query，Run state 與 Evaluation 寫入仍和 trace event 同交易；`Service.MaskingActivity` 以自身 Pool 回三個 owner count facts，API／worker roots 將同一 Trace Service 注入 Run。recent／since 雙窗與 P1 halt 判定不變，缺 transaction／Pool／Trace dependency 明確回錯，不 panic 或偽裝零值。

2026-08-22（DDD-060 實作註記）：Identity quota 的交易型 owner read 不再發布 query-shaped executor；`ReadWorkspaceCreatedAt` 只接 caller `pgx.Tx`，Identity 內才建 sqlc query。Run 在 `requireRunSlot` 後沿同一 transaction 建自己的 run-count query並呼叫 Identity owner read，因此 advisory lock、未提交可見性與配額判斷順序不變；`QuotaFor` 仍使用 pool-backed Identity Service method，缺 transaction 明確 fail closed。

| 組 | 內容 | 成因 | 清除方向 |
| --- | --- | --- | --- |
| **A** | ~~registry 的 Skill／Version 事實（原 21 組 caller pair）~~ | `skills`／`skill_versions` 是全平台的共享事實；Registry 原先只公開寫入面 | **DDD-043～047 已全數收斂**；consumer DTO callbacks 取代直接 sqlc，`read_allow:` 歸零 |
| **B** | ~~`LockSkillForRestriction`（catalog）、`LockTestCase`（run）~~ | 寫入已回到 owner，**同一筆動作的 `SELECT … FOR UPDATE` 沒跟著搬** | ~~鎖併進 owner 的寫入函式。**優先收，見下**~~ → **2026-08-21 DDD-031 已收**，見下 |
| **C** | ~~testlab 的 Test Case／Dataset 快照（6 條）~~ | 與 A 同因，但成本低得多：附錄 A 已允許 `run`／`eval`／`packaging` → `testlab` 三條 import，testlab 也已有 Service 門面（DDD-007） | ~~直接走既有 import ＋ Service 門面。`ListWorkspaceObjectKeys`（identity 的帳號刪除盤點待刪物件鍵）方向與 ADR-034 的 `PurgeWorkspace` 一致，併進那支注入~~ → **2026-08-21 DDD-033 已收**，見下（`ListWorkspaceObjectKeys` 沒有併進 `PurgeWorkspace`，理由見下） |
| **D** | ~~run 的 Run 事實（原 6 個條目、7 組 caller pair）~~ | 與 A、C 同因，但 import 方向被 depguard 擋著 | **DDD-040～042 已全數收斂**：依用途注入 owner DTO/read callbacks，沒有新增反向 import |
| **E** | ~~objreconcile 代讀 owner 的清單（3 條）~~ | DDD-020 反轉了掃描器的**寫入**半邊，**讀取半邊留著** | ~~補 owner lister 注入~~ → **2026-08-22 DDD-035 已收**；owner DTO 止住 generated row，composition root 轉換，五個 dependency fail closed |
| **F** | ~~單點跨界（原 6 條）~~ | 各有各的成因 | → **DDD-036～039 已全數收斂**；quota 規則仍屬 policy，但其兩個資料事實各由 run／identity owner API 回答 |
| **G** | ~~trace 事件的讀取（2 條）~~ | 與 C 同型：import 方向已合法（附錄 A 有 `run → trace`、`eval → trace`），trace 也已有 Service（DDD-004） | ~~純粹還沒搬，成本同 C~~ → **2026-08-21 DDD-033 已收**，見下 |

### B 組優先，因為它是正確性問題

`LockSkillForRestriction` 與 `LockTestCase` 都是 `SELECT … FOR UPDATE`（`normalizeSQL` 會把 `FOR UPDATE` 消掉，所以本檢查正確判為 read）：呼叫端先鎖 owner 的列，再叫 owner 寫。DDD-020／ADR-034 反轉的是**寫入那一半**，鎖這一半沒跟著搬，於是「鎖誰」這個決定仍在呼叫端手上——**owner 無法保證自己的寫入被正確序列化**。第二個呼叫端只要鎖錯行、或忘了鎖，owner 這邊不會有任何跡象。

這與其他六組不同級：A／C／D／E／F／G 是整潔與 schema 耦合問題（今天不收，明天也只是更難改）；B 是**正確性**問題，而且是已經被反轉一半、留下不完整接縫的那種——比從未動過的更危險，因為它看起來已經收過了。

#### 2026-08-21（DDD-031 實作註記）：B 組已收

上面的判斷不變，兩條都照它收掉了——**鎖搬到寫入所在的 context，不是搬到別的地方**：

- `LockSkillForRestriction`：`registry.SetAccessRestriction` 自己取 `FOR UPDATE`，並把 before-state 當作回傳值交出去；catalog 不再自己讀那一列，它拿到的是 owner 保證過的前值。簽章由 `(...) error` 變成 `(...) (gen.LockSkillForRestrictionRow, error)`。catalog 保留了它原本擁有的每一項決定：reason code 清單、兩條 HTTP route、授權檢查、audit event。
- `LockTestCase`：`testlab.CreateSnapshot` 自己取鎖（原本讀的是不上鎖的 `GetTestCase`，完全倚賴呼叫端先鎖），而 run 需要的「臨界區要在權限確認**之前**開始」則由新的 owner 匯出函式 `testlab.LockDraft(ctx, q, workspaceID, testCaseID)` 提供（**刻意不與 query 同名**：本檢查以 `.LockTestCase(` 文字比對找呼叫點，同名 wrapper 在每個呼叫點都與 query 無從分辨，等於把剛清掉的違規原樣種回去——見「已知盲點」第二條）。同一交易內重複鎖同一列在 Postgres 是 no-op，所以兩者並存不是成本。`run → testlab` 本來就是附錄 A 的合法方向，故不需要 ADR-034 式的注入。

兩者都維持在**呼叫端的交易內**（收 `pgx.Tx`／`*gen.Queries`，不自開交易），鐵律 9 不變。錯誤語意逐條保留：`pgx.ErrNoRows` → owner 的 `ErrNotFound` → run／catalog 的「找不到」，軟刪除與跨 workspace 的列仍讀為找不到。

守門的是三樣東西，缺一不可：`read_allow:` 兩條移除後，任何呼叫端再直接叫這兩條 query，`automation-check` 就是紅的（新違規）；`internal/skill/library/restriction_test.go` 以兩個真實交易競爭同一列，斷言第二個操作者看到的 before-state 是第一個 commit 後的值（拿掉 `FOR UPDATE` 即紅）；`internal/entrypoint/api/apiserver/snapshot_lock_integration_test.go` 讓一筆併發編輯與 `CreateSnapshot` 競爭，斷言凍結會等、且凍到 commit 後的 prompt（把鎖換回 `GetTestCase` 即紅）。

刻意沒做的事：A／C／D／E／F／G 六組原地不動——它們是整潔問題，本批只收正確性那一組；`testlab` 沒有新增資料庫測試骨架（那會是第四個重置 `public` schema 的套件），case 2 的併發測試因此放在既有的 `apiserver` 整合測試裡。

### 2026-08-21（DDD-033 實作註記）：C 組與 G 組已收

兩組的成因相同，也照上表的方向收掉了：**owner 沒有開讀取面，所以呼叫端各自去叫 query**。九個呼叫點全部改成呼叫 owner 匯出的函式，**沒有任何一個決定跟著搬家**——這批搬的是取事實的機制，不是業務規則。

C 組（testlab），四支新匯出函式，全部收呼叫端的 `*gen.Queries`、不自開交易（鐵律 9 不變）：

| 函式 | 取代 | 呼叫端 |
| --- | --- | --- |
| `testlab.ReadSnapshot(ctx, q, workspaceID, snapshotID) (gen.TestCaseSnapshot, error)` | `GetTestCaseSnapshot` | `eval/comparison.go`、`eval/eval.go`、`run/schedule.go` |
| `testlab.ReadDataset(ctx, q, workspaceID, datasetID) (gen.Dataset, error)` | `GetDataset` | `run/grants.go` |
| `testlab.CasesForSkill(ctx, q, workspaceID, skillID) ([]gen.TestCase, error)` | `ListTestCasesForSkill` | `packaging/testcase.go` |
| `testlab.CaseDatasets(ctx, q, workspaceID, testCaseID) ([]gen.Dataset, error)` | `ListDatasets` | `packaging/testcase.go` |

`ReadSnapshot` 回傳資料列而不是解碼後的視圖：三個呼叫端要的是三個不同子集，欄位怎麼讀已經由 `DecodeCriteria`／`DecodeRubric`／`DecodeDatasetRefs` 回答過一次，再包一層等於把同一組欄位定義寫第二遍。錯誤語意逐條保留：`pgx.ErrNoRows` → `testlab.ErrNotFound`，三個呼叫端本來就沒有區分「查無此列」與真錯誤（都往上丟成 500），現在也沒有。

**Object-key reader 沒有併進 `PurgeWorkspace`**，理由是兩半根本不在同一個時刻跑：物件儲存沒有 rollback，所以帳號刪除**先刪物件、後開交易刪列**（`identity/purge.go` 的既有註解就是這麼寫的）。一支在交易裡執行的 `PurgeWorkspace` 沒辦法回答呼叫端在交易開始**之前**才用得上的問題。因此它走的是同一種注入、但另一組欄位：

- `testlab`／`run`／`packaging` 各自公開 `WorkspaceObjectKeys(ctx, q, workspaceID)`，由 composition root（`apiserver.NewApp` 與 `cmd/maintenance`）注入 identity 的三個具名欄位。三條 SQL 都以 `workspace_id` 加各自的資料種類收窄；identity 在交易前合併並去重。
- **`identity → testlab` 確實是 deny**（`apps/platform/.golangci.yml` 的 `identity` 規則第九條），所以這裡本來就只有注入這條路，ADR-034 的判讀在這一條上成立。
- 未注入即**拒絕整批**（`requirePurgeSteps` 多一條檢查）。理由與既有五個 step 相同、而且更直接：少了它，列刪光了、使用者上傳的檔案還在，而該次執行照樣回報成功。

**2026-08-22 已收掉原先的髒角落**：`ListWorkspaceObjectKeys` 已拆為 dataset、run output 與 download package 三條 owner query；原本不分 `kind` 的 `DeleteWorkspaceArtifacts` 也同步拆為 run／packaging 兩條。物件仍先刪、資料列仍在 identity 擁有的單一 transaction 內全有或全無，沒有為了 ownership 犧牲 CORE-007 的原子性。

G 組（trace）兩支：

| 函式 | 取代 | 呼叫端 |
| --- | --- | --- |
| `(*trace.Service).LiveEvents(ctx, workspaceID, runID, eventIDs) ([]pgtype.UUID, error)` | `FindLiveTraceEvents` | `eval/http.go`（eval 已注入 `Trace *trace.Service`，不必再接線） |
| `trace.MaskingActivity(ctx, q, recent, since) (gen.CountTraceMaskingInWindowRow, error)` | `CountTraceMaskingInWindow` | `run/halt.go` |

`MaskingActivity` 是套件層函式而不是 Service 方法，因為 `run` 手上沒有 `trace.Service`（它只有 `trace.Signer`），而既有的 `trace.RecordOrchestratorEvent` 已經是「收呼叫端 `q` 的套件層函式」這個形狀。窗口長度（`maskingWindow`／`maskingConfirm`）**刻意留在 `run/halt.go`**：多少流量算證據、條件要成立多久、成立了要做什麼，都是 run 的判定；trace 只回答「遮罩器做了什麼」。

**命名全部刻意避開 query 名**（`ReadSnapshot` 而非 `GetTestCaseSnapshot`，`CaseDatasets` 而非 `ListDatasets`，`LiveEvents` 而非 `FindLiveTraceEvents`…）。這不是風格：本檢查以 `` `.QueryName(` `` 文字比對找呼叫點，同名 wrapper 在每個呼叫點都與 query 無從分辨，等於把剛清掉的違規原樣種回去——DDD-031 已經踩過一次，見「已知盲點」第二條。

守門的東西：`read_allow:` 七條移除後，任何非 owner 再直接叫這七條 query，`automation-check` 就是紅的（新違規）——**「呼叫點搬回去」這一半由它守，沒有再寫成 Go 測試**。語意漂移那一半由測試守：`apiserver/read_face_integration_test.go` 直接對七支新函式斷言 workspace scope 與找不到的答案，`governance_integration_test.go` 改成逐鍵比對（不再只數兩個），`evaluation_integration_test.go` 第一次斷言 evidence 的 `available`（在此之前整個 repo 沒有任何一處斷言它）。

刻意沒做的事：A／D／E／F 四組原地不動；`db/queries/*.sql` 一行沒改（包括上面那個 UNION 的髒角落）；`testlab`／`trace` 沒有新增資料庫測試骨架（那會是第四、第五個重置 `public` schema 的套件），所以新測試放在既有的 `apiserver` 整合測試套件裡。

### D 組的正解是注入，不是 import

`eval → run`、`trace → run`、`analytics → run`、`policy → run` 全都在 depguard 的 deny 清單裡，而 `run → policy`、`run → trace` 是附錄 A 的既有合法方向。把 D 組改成 import 會**製造編譯期循環**，Go 直接拒絕。

這與 [ADR-034](./ADR-034-cross-context-writes-close-by-inversion-not-by-events.md)「為什麼是注入而不是 import」同源：循環是訊號不是障礙，它說的是這些 context 互為對等（peer）而非上下層——`run` 需要 `trace` 寫事件、`trace` 需要 `run` 的 Run 事實，誰也不在誰底下。依賴反轉是對等關係的標準表達，且本 repo 已有三次前例（DDD-020 的 objreconcile、ADR-034 的搜尋投影與帳號刪除 purge）。

## 考慮過但拒絕

- **read 只宣告不擋，維持 ADR-033 現狀**：ADR-033 的理由（訊號稀釋）已隨 `allow:` 清空而失效。留著等於讓「暫時」變成事實上的永久，而 read 的存量正在長——A 組的 21 條就是「沒人擋所以每個 context 各接一條線」的直接結果。
- **read 一律禁止、不設容忍段落**：需要先重構 56 處呼叫點才能讓 CI 變綠，而重構 `internal/` 與本批的目的（補檢查）無關，也會與正在改那裡的其他工作衝突。棘輪的意義就是「存量凍結、增量歸零」。
- **為 read 另立一套 Generic 語意**（例如讓 Generic 套件自由讀所有 context）：E 組正是反例——Generic 掃描器自己決定「哪些列該被掃」，那是領域決定不是機制。「Generic」指的是沒有領域規則，不是可以繞過所有權（ADR-033 決策 5 的原話）。
- **把對照表完整性做成獨立的 Taskfile task 或 lint plugin**：同決策 4。

## 影響

### 正面

- 「誰在讀誰的表」第一次有了單一、可被 CI 驗證的答案。35 條從隱形變成有名字、有成因分組、有清除方向的待辦。
- 新的跨 context read 在 CI 就被擋下，A 組不會再長。
- ADR-032 §1 對照表從「讀過 ADR 的人才知道要登記」變成「漏登記 CI 會紅」，AGENTS.md 第 11 條第一次有強制力。
- 不動 `db/sqlc.yaml`、`db/queries/*.sql`、任何 generated 目錄或 `apps/platform/internal/**`，沒有 codegen drift 風險。

### 代價

- 新增跨 context read 的成本從零變成「改程式或過一次 ADR」。這個摩擦是刻意的。
- `read_allow:` 的 35 條會在清除期間反覆被編輯；每次移除都要確認呼叫點真的沒了（失效即 FAIL 幫忙守這一半）。
- 對照表檢查以正規表示式讀 markdown 表格。§1 的欄位順序或反引號慣例若變動，檢查會報「讀不出來」而不是靜默通過——這是刻意選的失敗方向，但它意味著改 §1 的格式要順手跑一次 `automation-check`。

## 不變事項

- ADR-032 §1 仍是 context 清單的事實來源；新增 context 要同時登記在 §1、`.golangci.yml` 與 `tools/devctl/query_owners.go` 的 `platformContexts`。
- ADR-033 的 owner 判準（**該 query 主要資料表的擁有 context**）不變。`CountQuotaRuns` 這類「表歸 A、用途歸 B」的張力維持按表歸屬——表是 `runs` 故歸 `run`，即使 ADR-032 §1 把 quota 規則判給 `policy`、而這條 query 除了算配額沒有第二個用途。改判會讓判準出現例外。
- ADR-034 的兩個交易保證、depguard 的跨 context import 規則、generated file 禁止手改、單一 Writer、所有實作鐵律，均不變。

## 待決策

- 存量七組清完後，是否拆 sqlc per-context package（ADR-033「考慮過但拒絕」），或把 `pgxpool.Pool` 收在只暴露 sqlc 的 wrapper 後面以封死裸 SQL 盲點。兩者成本同級，且都要等存量清完。
- ~~ADR-032 §1 對 `skillpkg` 的兩種讀法（Core 列的成員 vs Shared Kernel／Generic）是否要統一。~~ → [ADR-037](./ADR-037-product-analytics-and-package-architecture-identities.md)：統一為 Shared Kernel，重複 identity 改為 CI FAIL。
