# Platform DDD 對齊調整計畫（2026-08-19）

## 1. 目的與分工

負責人裁示：platform 層未來會持續吸收領域知識與變動，MVP 之後必須以 DDD 風格治理架構，避免失控。本計畫是 [ADR-032](../adr/ADR-032-ddd-bounded-context-governance-for-platform.md)（Proposed）的執行面工作清單。

與 [技術債總帳](./technical-debt-remediation-2026-08-19.md) 的分工：債務帳管**正確性與安全**（單點缺陷），本計畫管**結構方向**（邊界與依賴）。兩份不重複收錄；有交集處以引用連結。

前提：本計畫全部項目都是 repo 內工作，不阻塞也不依賴 `RELEASE-001`～`010`；Phase 0 可與部署期並行，Phase 1 起建議在 MVP 收尾後執行。

## 2. 現況診斷（2026-08-19 架構審查的 DDD 相關結論)

戰略設計早已存在——ADR-002 的九個領域模組就是 Bounded Context，且 `internal/` 的切分依領域而非技術層，方向正確。失控風險不在「沒有 DDD」，在**邊界從未被機器強制**，已漂移四類：

1. `internal/ingest` 與 `internal/testlab` 身分不明：既是領域模組，又被 4 個 context 當 library import（`eval/apply.go`、`run/gateb.go`、`run/service.go`、`run/schedule.go`、`packaging`、`catalog`）。
2. 依賴方向反轉：`run/service.go:526` import `eval` 只為 `eval.JobArgs` 入隊。
3. DI 逃逸：`eval/eval.go:375`、`eval/comparison.go:229` 現場 `&trace.Service{Pool: s.Pool}`，`Signer` 恆 nil。
4. 層次不一致：`catalog` 無 Service 層（`catalog/http.go:32` 直持 pool）；composition root 在 `cmd/api/main.go:39-228` 與 `identity/authz_integration_test.go:250-342` 有兩份手抄本。

另兩項放大失控風險的既有事實：`internal/run` 已達 5,663 行 11 檔（狀態機、排程、dispatch、preflight、quota、gateway、halt、cleanup、supervisor、grants 同居）；整合測試 11.5k 行寄居 `internal/identity`，可發現性差。

## 3. 調整項目 ledger

狀態沿用債務帳慣例：`open`／`fixed`／`decision`。`fixed` 必須同時有實作、回歸測試與 CI 檢查。

### Phase 0 — 止血（先擋新增漂移，不搬存量）

| ID | 狀態 | 項目 | 完成條件 |
| --- | --- | --- | --- |
| DDD-001 | fixed | ADR-032 評審定案（負責人） | **2026-08-20 完成**：負責人授權執行並委任最佳實務決策（含三項待決策的 08-19 裁定），ADR-032 轉 Accepted。定案時依實測 import graph（Docker `go list`）修正三處：補列 Run Trace context、補「run → trace」白名單列、更正 `llmclient` 使用者清單。 |
| DDD-002 | fixed | depguard 白名單凍結現況 | **2026-08-20 完成**：`apps/platform/.golangci.yml` 啟用 depguard，12 條規則＝ADR-032 附錄 A 機器版（含 `!$test` 排除，整合測試由 DDD-011 收斂）；drift 標記 DDD-005×1、DDD-006×4 與 ADR 一致；`devctl automation-check` 新增兩側 drift 標記多重集合比對（含單元測試）。驗證：現況 lint 零 depguard 違規；假違規探針檔實測被抓（`import ... not allowed from list 'registry'`）後移除回綠。 |
| DDD-003 | fixed | 抽 `apiserver.NewApp(Config)` 唯一 composition root | **2026-08-20 完成**：`internal/apiserver/app.go` 新增 `Config`／`App`／`NewApp`／`Handler()`／`AuditRosters()`；`ObjectStore` 以四個既有 per-context interface 的 embedded union 定義（不會與成員分岔）。`cmd/api/main.go` 385→279 行、不再 import 六個領域套件；`newAPITuned` 手抄物件圖刪除（roster fail-closed 邏輯與 `BetaGateClosed()` 移入 apiserver 並有單元測試）。附帶效果：測試 wiring 與生產更一致（`ingest.LLM`、`registry.Store`、analytics `Secure` 三處原本測試側未接，現已接上）。驗證：lint 0 issues、單元 289 PASS、整合（拋棄式 pgvector）497 PASS／0 FAIL。 |
| DDD-004 | fixed | 修 DI 逃逸 | **2026-08-20 完成**：`eval.Service` 加 `Trace *trace.Service` 欄位（註解載明 ADR-032 §5），`eval/eval.go`／`comparison.go` 的現場建構移除；注入點：NewApp（與 Trace handler 同一實例，`Signer` 組態完整）、`cmd/worker/main.go`（重用 dispatcher 既有 signer）、兩個直接建構 `eval.Service` 的整合測試。刻意**不加 nil fallback**——漏接會 panic 而非靜默降級。 |

### Phase 1 — 邊界糾偏（清除附錄 A 的 drift 標記）

| ID | 狀態 | 項目 | 完成條件 |
| --- | --- | --- | --- |
| DDD-005 | fixed | `run` → `eval` 事件化 | **2026-08-20 完成**：`eval.RunEventConsumer` 消費 `run.succeeded`／`run.failed` 入隊 `evaluate_run`，終態交易只入 `run_cleanup`；`run` 不再 import `eval`（depguard 已 deny）。冪等採 **guard 查詢**（`GetCurrentEvaluation`——evaluation 列永存，比擴 UniqueOpts 狀態集正確：後者會連 ADR-026 的刻意重評一起擋掉且隨 River retention 失效；理由註記於 consumer，關聯 LLM-EVAL-005）。outbox deliver 失敗改為 log＋回錯讓 River 重試（原吞錯已除，目錄 §5 缺口 2 同步改寫）。新增端到端整合測試「終態→publish→consumer→恰好一筆評估＋重送不加」。驗證：lint 0、無 DB 測試綠、整合全綠。附帶發現一個**既存 flake**：`TestAdvancedViewNamesMissingEventsAndRefusesToLookComplete` 同批 trace 事件 `occurred_at` 相同導致排序不定（與 TRACE-SEQ-001 同根，未修，在 allowlist 外）。 |
| DDD-006 | fixed | `ingest` 拆分（**裁定於執行時調整，理由如右**） | **2026-08-20 完成**：盤點證實 catalog／run／packaging 對 ingest 的依賴只有無狀態純函式（`PackageFS`／`PackageRoot`／`MaxZipBytes`＋唯一 producer 的 `ErrBadArchive`）——已移入 Shared Kernel `skillpkg`（`archive.go`，17 個引用點改乾淨、無 alias），三條依賴消滅並移入 depguard deny。`SaveVersion` 內部是完整 Trust 驗證管線，且 M4 PACK-002 已裁定「採納建議重用匯入驗證路徑」——搬進 registry 會製造第二條版本建立路徑（第二個真相），故 **`eval` → `ingest` 合法化**為 Customer–Supplier 邊（非原計畫的「不再 import」）。ADR-032 §1／附錄 A 與 ADR-002 待決策回填已同步；兩側 drift 標記歸零。驗證：lint 0（三條新 deny 生效）、無 DB 測試綠、整合全綠（trace flake 未發）。 |
| DDD-007 | fixed | `testlab` 公開契約面依 UI/UX 導向重整（設計見 [testlab-contract-design-2026-08-19.md](testlab-contract-design-2026-08-19.md)） | **2026-08-20 完成（007a platform＋007b web 兩批）**。契約面 B：`testlab.ReadDraft` 讀取門面落地，`run/preflight.go` 兩處直讀 `gen` 消滅、`DatasetSummary` 重複型別刪除（permission summary JSON 位元組不變，既有確認不作廢）；packaging 改走 `DecodeCriteria`；`ReadSnapshot` **裁定不建**——三個快照讀取點已經走 `Decode*`，包一層是無缺陷的抽象。契約面 A 全數落地：skill 篩選＋列表彙總、`runs?test_case_id=` 回程（in-Go 500 筆掃描附 `ponytail:` 升級路徑）、執行歷史區＋版本預設、刪除使用者面（WS-002 刪除範圍揭露）、`expires_at` 上畫面、suggest 改「回傳建議→逐條採納（`source:suggested`）」、preflight 進階收合區（hash 涵蓋欄位全數可見）。疑點結案：`estimated_cost` **本來就有宣告**，過時的是 m2/README 註記（`02` 已同步修正）。驗證：redocly 基線、gen 三語言 current、platform lint 0＋整合全綠、web tsc（--force）／oxlint／prettier／vitest 133／build 全綠。 |
| DDD-008 | fixed | `catalog` 補 Service 層 | **2026-08-20 完成**：`catalog.Service{Pool, LLM, Store, Analytics}`＋`Handler{Svc, Identity}`，與其他九個 context 同構；檢索管線（降級階梯、match-reasons、O11Y 直方圖）、detail 組裝、restriction 交易全數移入 Service（**搬移非重寫**，決策註解全保留，endpoint 行為／SQL／排序零變更；scope 檢查仍在 Handler——所有權不動）。`http.go` 915→642 行。驗證：lint 0、無 DB 綠、整合 14 套件全綠。 |
| DDD-009 | fixed | 共用 helper 收斂進 `platform/` | **2026-08-20 完成**：新增 `platform/pgconv`（`UUIDString`／`RFC3339`／`Timestamptz`，含單元測試）、`platform/envx.Or`、`objstore.FromEnv()`；刪除 18 份複本（uuidString×11、rfc3339×2、pgTime×1、envOr/addrFromEnv×4），47 檔淨 −82 行。等價性逐份驗證（含對照 pgx v5.10.0 原始碼確認防禦分支是死碼）。**一份刻意不收**：packaging/manifest.go 的 `rfc3339` 對 NULL 回 epoch 而非空字串——manifest 位元組受 ADR-027 雙雜湊約束，統一＝改變可重現性，留在原地附註解（若日後判定 epoch fallback 非刻意，需連同 manifest 版本一起決策）。驗證：lint 0、無 DB 綠、整合 15 套件全綠。 |

### Phase 2 — 結構深化（規模成長前的預整）

| ID | 狀態 | 項目 | 完成條件 |
| --- | --- | --- | --- |
| DDD-010 | fixed | 拆 `internal/run` 內部結構 | **2026-08-20 完成（採檔案群，非子包——子包迫使大量 export 與潛在循環，成本不成比例）**：新 `statemachine.go`（373 行，aggregate 規則＋唯一寫入路徑 `Transition`/`record` 合體，原 `state.go` 併入）、`service.go` 892→642（純應用面）、`doc.go` 四群職責地圖、`provider.go`/`gateway.go` 補 ACL 檔頭。狀態機測試：9×9 全矩陣（20 條合法轉移逐一斷言）、終態死路、`AllStatuses` 對 enum 的覆蓋防護（DB 加狀態沒登記會紅）。**發現未修**：`job.go:423` `settle` 依賴轉移表每列第一個元素是 happy path——順序 load-bearing 但無測試釘住，後續宜加 `next(status)` accessor＋測試。驗證：lint 0、無 DB 302 pass、整合 516 pass。 |
| DDD-011 | fixed | 整合測試搬家＋AuthZ 矩陣補全 | **2026-08-20 完成**：23 檔整批 `git mv` 至 `internal/apiserver`（`package apiserver_test`；零檔留 identity——三個 session 生命週期測試也走 HTTP 表、依賴同一 harness，留下＝複製兩份 `DROP SCHEMA` 的 TestMain 互踩）。矩陣 30→**68 條全表**（router.go 61＋identity.Mount 7），語意逐條斷言（401/404/200/204、`/me/quota` 兩態、trace ingest 的 token-401）；**雙向自我強制**：測試解析 router.go＋identity/http.go 的 mux.Handle pattern 與矩陣比對，掛了沒進表、表有沒掛都紅（故意破壞實測兩方向都點名），pattern 解析不了會 Fatal 不會靜默漏。另補不需 DB 的路由表煙霧測試。CI 的 immutability bootstrap 行同批改指 apiserver（agent 已實測 39 張表建出）。驗證：lint 0、無 DB 301 pass、整合 515 pass 零 flake。 |
| DDD-012 | fixed | 事件目錄程式側收斂 | **2026-08-20 完成**（migration 為 **0035**，0032 已被佔用）：缺口 1/2/3/4/5/7 全關——retention（`PublishedRetention` 預設 7 天，每輪 publish 先修剪、dead 列永不刪）、poison 最小隔離（attempts＋`dead_lettered_at`＋閾值 10＋metric＋backlog 解鎖；無自動重放，如實記於目錄）、causation 規則精修為**兩類 NULL 豁免**（genesis 事件；直接成因無 UUID 身分者——cleanup 的成因是 River job（bigint），硬塞 attempt id 會改 `ByArgs` 去重鍵造成雙 worker 拆同一 sandbox）、`AggregateRun`／`EventVersion1` 常數化、`outbox.Insert(ctx, tx pgx.Tx, ...)` 編譯期強制同交易、11 值封閉集合（常數＋switch 映射拒絕未知 status＋DB CHECK＋三方 conformance test——test 以假常數探針證明非空轉）。缺口 6（aggregate version）依裁定保留 open。驗證：build/vet 綠、lint 0、conformance 4/4、整合全綠（0035 schema 實查確認、identity 222 測試連跑兩輪無 flake）。 |
| DDD-013 | fixed | Ubiquitous language 固定進 AGENTS.md | **2026-08-20 完成**：AGENTS.md 開發自動化守則新增第 11 條——context 速查表（標明事實來源為 ADR-032 §1）、新增套件先登記、跨 context import 同 commit 改兩處、Service 一律經 NewApp 注入；`devctl automation-check` 驗證通過。 |

### Phase 3 — DDD 化完成後（負責人 2026-08-19 裁定時點）

| ID | 狀態 | 項目 | 完成條件 |
| --- | --- | --- | --- |
| DDD-014 | fixed | 抽離 Policy & Usage context | **2026-08-20 完成**（前置 Phase 0～2 同日收斂）：新 `internal/policy`（QuotaLimits／EnforceQuota／Usage／DownloadRetention，含單元測試與計費接縫定位）；`run`、`packaging` 為兩個合法 Customer–Supplier 客戶（depguard 新 policy 規則＋10 條既有規則 deny 更新，drift 0=0 不變）。**語意零變更經套件與定向測試雙重驗證**：ADR-028 強制點仍在 create-run 交易內（policy 決定、run 執行）、三個拒絕標籤位元組不變、`RUN_QUOTA=off` 雙關閉不變、GOV-RETENTION-001 的 fail-closed 不變。刻意留在原地：`MaxConcurrentRunsPerWorkspace`（Run Orchestration 的 in-flight 不變量，非 allowance）、env 解析（cmd）。驗證：lint 0、無 DB 304、整合 518 全綠。 |

### Phase 4 — 審視報告後續（2026-08-20 架構審視）

> 來源是一份唯讀架構審視報告（檢查 commit、ADR、import graph、composition root、Outbox、測試與 CI 設定），**刻意未納入版本控制**——它是某一個時點的意見，不是需要被維護的基準；其結論已逐項落進下表與 §6，此後以本 ledger 為準。

Phase 0～3 收斂後的唯讀審視認定方向正確，但**資料與事件邊界尚未完整封裝**，列三項風險：單一 sqlc package 是跨 context 的資料存取後門（P1）、`NewApp` 不是整個 platform 的唯一 composition root（P1/P2）、Outbox 是單一 callback 而非可擴充 dispatcher（P2）。本 Phase 逐項處理，並補上 ADR-032 §4 三個 aggregate 中未做的兩個。

| ID | 狀態 | 項目 | 完成條件 |
| --- | --- | --- | --- |
| DDD-016 | fixed | Query ownership 機械強制（審視 P1） | **2026-08-20 完成**：`db/query-owners.yaml` 宣告 171 條 query 的 owner context（18 檔預設＋44 條覆寫），`devctl automation-check` 擋下**跨 context 的 write query 呼叫**（read 只宣告不擋，理由見 ADR-033）。呼叫點認定＝「import 了 `db/gen` 的非測試檔」，一個條件同時排除 `api/gen`、`db/gen` 自身與整合測試。雙向完整性：未宣告的 query、殘留條目、已無呼叫點的 stale allow 三者皆 FAIL。**刻意拒絕拆 sqlc per-context package**（理由見 ADR-033「考慮過但拒絕」）。導入時量到 15 條存量跨 context write，全數列入 `allow:` 標 `drift: DDD-015`，未順手重構。新增 [ADR-033](../adr/ADR-033-sqlc-query-ownership-and-cross-context-write-enforcement.md)。 |
| DDD-017 | fixed | Outbox 改宣告式路由（審視 P2） | **2026-08-20 完成**：`outbox.Dispatcher`（`On`／`Ignore`／`Validate`／`Deliver`）。`EventTypes` 每個成員必須被明確認領或明確放棄，缺一則 `Validate` 在 composition root 回錯、process 開機失敗；未路由的 event type 在 `Deliver` 回錯而非 no-op，所以漏叫 `Validate` 也不會被標 published。**移除 production 的 silent log fallback**：`Deliver == nil` 不再退化成 log，log-only 需明示 `LogOnlyDelivery`，且該判斷在取 pool 與上鎖之前完成（可不依賴 DB 測試）。七個測試含審視點名的那條：Evaluation wiring 缺失時 `run.succeeded` 不會被當成功消費。**刻意不做**：per-consumer offset／replay cursor（需新表，等第二個 consumer 且重試成本不同再說）。 |
| DDD-018 | fixed | 四個 process、四個 composition root（審視 P1/P2） | **2026-08-20 完成**：`cmd/api` 與 `apiserver.NewApp` 原本都宣稱 NewApp 是 platform 唯一 root，實際 worker／maintenance／reindex 各自建構 Service。NewApp 正名為 **API 的** root，四個 `cmd/*/main.go` 各自寫明自己的 root，ADR-032 §5 依既有前例追加實作註記（決策文字未改寫）。worker 的建構抽成無 I/O 的 `buildWorkers`，配 `main_test.go` 四個 wiring smoke test——其中一個守住 `run.Service.Queue` 漏設（該 bug 真實發生過，曾使每個 run 完成後未清理）；apiserver 加反射式 `App.Deps` wiring 測試。兩者皆做 mutation check。**刻意不抽共用 wiring factory**：唯一重複的是 env 讀取，三個 process 對缺值的處置與警告訊息各異，抽出去要加旗標還原差異——程式更多、log 更差。maintenance／reindex 不寫 smoke test：wiring 是單一 struct literal 緊接唯一使用它的呼叫，漏欄位當場在 operator 終端失敗。 |
| DDD-019 | fixed | 清除路徑 2：`ingest` → `registry` 寫入 | **2026-08-20 完成**：`registry` 新增 `CreateSkillFromPackage`／`CreateVersionFromPackage`／`UpdateSummaryFromPackage`，三者**收呼叫端的 `pgx.Tx`、不自開交易**——匯入的版本列、搜尋投影（INGEST-009 要求同交易）與 audit 事件仍在同一個 commit（鐵律 9）。參數收 `skillpkg.Report` 而非散裝字串，決定寫什麼的輸入即驗證產物；guard 只擋明顯偽造（blocked、無 manifest）且 doc comment 誠實載明無法證明管線跑過，未發明誰都能構造的憑據型別。ADR-021 的 license 表述／來源分離與 manifest JSON 編碼一併移回 registry（manifest 位元組自此單一來源）。`ingest` → `registry` 同批移出 depguard deny 並加入 ADR-032 附錄 A。`allow:` 15 → 12。 |
| DDD-020 | fixed | 清除路徑 4：掃描器回報、owner 寫入 | **2026-08-20 完成**：`objreconcile` 是 generic 掃描器卻直接寫 packaging 的 `artifacts` 與 testlab 的 `datasets`。兩個寫入改為 `packaging.MarkArtifactPurged`／`testlab.MarkDatasetObjectLost`，由 composition root **注入**而非 import——generic 反向依賴領域套件是把 ADR-032 的分層倒置，故**該半邊 depguard 與附錄 A 零改動**。`Sweep` 缺注入即回錯，且擋在 `Store == nil` 的 early return **之前**（無 store 是合法部署，漏注入是 wiring bug，不得躲在它後面）。注入欄位刻意命名 `RecordArtifactPurged`／`RecordDatasetLost` 而非 query 名——ownership 檢查是文字比對，同名會被誤判成仍在呼叫該 query（ADR-033「代價」章節預告的陷阱，此為首次撞上）。catalog 寫 `skills.access_restriction` 改走 `registry.SetAccessRestriction`（收 catalog 的 tx），reason code、說明句、兩條路由、授權與 audit 全留 catalog——`restriction.go` 檔頭既有的反對論證未被推翻，而是在其下補述新切法為何不觸發它。`allow:` 12 → **9**。 |
| DDD-021 | fixed | Skill Version aggregate 補文件與測試（ADR-032 §4） | **2026-08-20 完成**：`registry/doc.go` 逐條寫下不變量（版本列永不 UPDATE／DELETE、採納建議＝INSERT、`version_number` 由 query 配號非呼叫端指定、三個快照欄位、license 表述與來源同行），並講明同 context 內 `skills` 可變而 `skill_versions` 不可變的分野。**執行時推翻了立項前提**：不可變性並非「碰巧沒人寫 UPDATE」——`0005_immutability.sql` 自 M0 即有 `enforce_immutable()` trigger，`db/tests` 有 `must_fail` 證明。缺的是**時機**：trigger 要等語句執行才炸（最快是 staging 的 500）。故改為 `db/query-owners.yaml` 新增 `immutable:`（八張表各附理由）＋`immutable_allow:` 具名豁免（一條，帳號刪除 purge），檢查擋四類事，其中一類是**宣告了但 migration 沒有對應 trigger**——兩邊互相對帳，單刪任一側皆 FAIL，順帶守住 trigger 被刪。`runs`／`evaluations` 刻意不納入：其 trigger 帶 WHEN 或欄位參數，是列級／欄位級凍結，納入會對正常的 `UPDATE runs SET status` 誤報。 |
| DDD-022 | fixed | Evaluation aggregate 補文件與測試（ADR-032 §4） | **2026-08-20 完成**：先釐清不變量再寫測試——「append-only」字面不成立（`evaluations` 有四條 UPDATE），`eval/doc.go` 因此**正面回答四條各自為何合法**：`SupersedeCurrentEvaluation` 是機制本身（只寫 `superseded_at`，同交易另一半是 INSERT）、`CompleteEvaluation` 是判定的第一次寫入（列先於 judge 呼叫建立，讓中途死亡是可見的 `pending`）、`FailEvaluation` 是同列的另一終態、`SetEvaluationFeedback` 是對判定的意見而非判定的一部分。九條不變量各自標明由誰強制，多數是 DB 而非應用紀律：`evaluations_current_key` partial unique index 保證「至多一份 current」、0024 trigger 凍結 completed 列、`status = 'pending'` 述詞讓 worker 與 recovery sweep 競賽而恰好一方勝出。**`superseded_at` 的一次性不是靠不可變性**——trigger 必須讓該欄可寫，擋住「還原成 current」的是 unique index。八個 DB 測試皆經破壞驗證。未搬檔：三條寫入路徑本已收在 `eval.go` 同一區塊。 |
| DDD-023 | fixed | Run aggregate 的順序依賴（§6 列管殘項第一條） | **2026-08-20 完成**：`settle` 原以 `successors[status][0]` 取 happy path 後繼，順序 load-bearing 而無人知曉，且外層迴圈無界——走不到 `succeeded` 是**無限迴圈**（每圈一次 DB 寫入），不是回錯值。新增 `NextOnSuccess`（從**集合**推導：唯一不是失敗出口的後繼，即轉移表註解本已陳述的規則，故重排任一列真正無作用，而非僅被測試釘住）與 `HappyPath`（先算完整路徑再套用；終態死路或超過 `len(AllStatuses)` 步回 `ErrNoHappyPath`，由 River 重試且可見）。轉移表內容與 9×9 合法性測試未動。三個新測試中兩個為外部（釘住確切生命週期路徑；釘住「每個非終態恰有一個非失敗後繼」——這是推導良定義的前提，加第二個就會紅），一個為內部（需抽換轉移表：全列反轉路徑不變、環狀表被拒而非迴圈）。皆經破壞驗證，移除步數上限會使該測試變成 hang。 |
| DDD-024 | fixed | 裸 SQL tripwire（ADR-033 的盲點） | **2026-08-20 完成**：ADR-033 全套機制的前提是「所有寫入經過 sqlc」，而該前提未被強制——一行 `tx.Exec(ctx, "UPDATE skills SET ...")` 同時繞過 ownership 與 immutable 兩道檢查。導入時洞是空的（`apps/platform` 八處裸 pgx 呼叫**無一為 DML**），為補防線最便宜的時刻。devctl 以 `go/ast`（stdlib，維持零第三方依賴）解析非測試 Go 檔，擋下交給 `Exec`／`Query`／`QueryRow`／`Batch.Queue` 的 DML 字面值；SQL 判定重用既有 `isWriteStatement`，不寫第二套 parser。入口列 `Batch.Queue` 而非 `SendBatch`（後者收 `*pgx.Batch` 不是 SQL）；只取第一個帶字面值的引數，避免 `Exec(ctx, sql, "delete")` 假警報。`raw_sql_allow:` 形狀比照另兩份清單，目前**零條**，失效豁免 FAIL。**明載為 tripwire 而非證明**：`const` 持有 SQL、`Sprintf`、字串串接、pgx 以外的路徑皆繞得過，且該形狀現況已存在（`run/halt.go` 的 `reconcilerLastRun`，是 SELECT 故無害）。真正封死需把 pool 收在只暴露 sqlc 的 wrapper 後——與「拆 sqlc per-context package」同級，同樣等存量清完再評估。 |
| DDD-025 | fixed | 清除路徑 3 改依賴反轉（[ADR-034](../adr/ADR-034-cross-context-writes-close-by-inversion-not-by-events.md)） | **2026-08-20 完成**：ADR-033 原提議把搜尋索引投影事件化，那正是「需要產品決策」的來源——最終一致會讓匯入成功後有一段搜不到的窗口，於是 M1 的可發現性允收準則得先鬆綁。**ADR-034 裁定不問這個問題**：ADR-032 §2 自己的判準把它判在同步側（匯入完成卻搜不到，是這筆請求沒被正確完成），且單一資料庫單一程序不是拿原子性換 ADR-010 尚未要求的解耦的地方。做法回到清除路徑 2／4 的形狀：`catalog` 公開 `IndexSkill`／`IndexSkillEnriched`／`RemoveSkillFromIndex`，三者收呼叫端的 `*gen.Queries`、不自開交易；`registry`（fork／delete／takedown）與 `ingest`（版本寫入／reindex 回填）**注入而非 import**——`catalog → registry` 自 DDD-020 起存在，`registry → catalog` 會是編譯期循環，而循環是「這兩者互為對等」的訊號。`ingest → catalog` 不成環仍採注入，避免同一組漂移用兩種形狀收斂。未注入即拒絕，驗證雙向：wiring 測試點名缺哪一條，fork 路由回 500 而非 commit 一個搜不到的 fork。`cmd/maintenance` 刻意不注入（其 `ingest.Service` 只跑 `CheckSources`，從不碰投影）。depguard 與附錄 A 零改動。`allow:` 9 → 6。 |
| DDD-026 | fixed | 清除路徑 1 改依賴反轉（ADR-034） | **2026-08-20 完成**：同一個裁定的第二組，也是最後一組。ADR-033 原提議各 context 訂閱 `account.deletion_due` 自清，那會讓 CORE-007 的硬刪除承諾需要重新定義「清完了」的判準與負責人。**ADR-034 裁定最終一致的刪除是被迫接受的、不是被選擇的**，刪除仍是單一交易的全有全無。五個 owner 各自公開 `PurgeWorkspace(ctx, q, workspaceID)`（analytics 的答案是去識別化，其 doc 說明理由）；`identity` 注入五個具名欄位並摺成**一份有序清單**，fail-closed 檢查與交易迴圈讀同一份，新增第六個 context 是一處一行。**順序 load-bearing 且被守住**：registry 必須在 ingest 之前（ingest 只移除沒有 `skill_versions` 指著的 source，而刪那些列的正是 registry），對調不會報錯也不會讓 row count 出錯，只是被刪帳號的匯入來源默默留著——整合測試種一個帶 source 的 version 並斷言 source 消失，對調即紅。`SET LOCAL skillhub.purge = 'on'` 是交易範圍，owner 步驟跑在同一交易內，故 registry 那步仍越得過不可變 trigger（若它哪天自開交易，資料庫會直接拒絕）。**原子性有常設測試**：末步換成回錯的 closure，斷言先前步驟的刪除**與 identity 自己的去識別化**都回滾——只斷言前者無法分辨「整批回滾」與「只跳過失敗那步」。`cmd/maintenance` 補 wiring smoke test（DDD-018 當時判斷其 wiring 太薄不需要，五個注入點加上「真正跑 purge 的是這個 process 不是 API」推翻了該判斷）。`allow:` 6 → **0**。 |

**Phase 4 的邊界**：審視報告第四項（`App.Deps` 與 Service handles 公開，架構規則靠慣例）**未執行**——審視自身評為「目前可以接受」，且整合測試大量依賴那些把手，改為 private 的 churn 大於收益。若日後要做，路徑是提供獨立 test constructor／test options，逐步縮小公開面。

### Phase 5 — read 側強制與 aggregate 文件面（2026-08-21）

Phase 4 把 `allow:` 清到 0 之後，ADR-033／034 各自留下的「read 何時強制」待決策失去了原本的阻擋理由（訊號稀釋來源已消失），順勢處理；同批補上 ADR-032 §4 三個 aggregate 之外其餘 context 的 package 文件，以及 §6 列管殘項的第三條。

| ID | 狀態 | 項目 | 完成條件 |
| --- | --- | --- | --- |
| DDD-028 | fixed | read ownership 開始強制，Context 對照表補完整性檢查 | **2026-08-21 完成**：新增 [ADR-035](../adr/ADR-035-read-ownership-enforcement-and-context-map-completeness.md)，回答 ADR-033 待決策第 1 項與 ADR-034 待決策第 2 項（兩處皆回填刪節線引用）。`db/query-owners.yaml` 新增 `read_allow:`，形狀與 `allow:`／`immutable_allow:`／`raw_sql_allow:` 完全相同（具名、有理由、失效即 FAIL）；判定式是唯一差別，解析與掃描完全重用，`tools/devctl` 維持零第三方依賴。**條目放錯段落也 FAIL 且不給豁免**——訊息直接指出該搬去哪一段。第二道檢查讓三份清單互相對帳：`apps/platform/internal/` 的套件目錄、ADR-032 §1 對照表、`.golangci.yml` 的 depguard 規則，任一方向缺漏即 FAIL——AGENTS.md 第 11 條的「新增套件必須先登記」自此第一次有機械強制力。對照表以正規表示式解析，格式變動會**報讀不出來而非靜默通過**（刻意選的失敗方向，代價是改 §1 格式要順手跑 `automation-check`，已在 ADR-032 §1 加實作註記）。存量 47 組 (query, caller)、56 個呼叫點，收斂為 35 條 `read_allow:`，分七群 A～G 各有清除方向。**B 組（`LockSkillForRestriction`、`LockTestCase`）被判為正確性問題而非整潔問題**：DDD-020／ADR-034 反轉了寫入半邊，`SELECT … FOR UPDATE` 沒跟著搬，於是「鎖誰」仍在呼叫端手上，owner 無法保證自己的寫入被序列化——看起來已經收過，反而比從未動過的更危險。D 組的正解是注入不是 import（`eval`／`trace`／`analytics`／`policy` → `run` 皆在 deny，而 `run → policy`、`run → trace` 合法，改 import 會製造編譯期循環）。**刻意不做**：read 一律禁止（要先重構 56 處才能綠，與補檢查無關）、為 Generic 另立可繞過所有權的 read 語意（E 組正是反例——掃描器自己決定哪些列該掃是領域決定）。 |
| DDD-029 | fixed | 其餘七個 context 補 package 文件 | **2026-08-21 完成**：ADR-032 §4 只要求三個 aggregate（DDD-021／022／023 已做），其餘 context 的邊界只存在於 ADR 裡，讀 `internal/` 的人看不到。`analytics`／`catalog`／`identity`／`ingest`／`packaging`／`testlab`／`trace` 各補 `doc.go`，逐份寫明所屬 context、需求 ID 前綴、不變量與各自由誰強制，並把散落在 `analytics.go`／`http.go`／`github.go`／`service.go`／`packaging.go`／`testlab.go` 的既有 package 註解**搬移**而非複製（同一份說明不留兩處）。零行為變動。 |
| DDD-030 | fixed | 失敗的評估記下它被嘗試時的條件（§6 列管殘項第三條） | **2026-08-21 完成**：`FailEvaluation` 只寫 `evidence_complete`，ADR-026 決策 1 要求的三個值留 NULL，而它們唯一的另一個家是 `evaluation_started` trace event——trace 以 DROP PARTITION 清除，保存期一過「那次失敗用的是哪個 judge」就消失。修法**不在 `fail()` 寫**（`recoverEvaluation` 是別的 process 的 sweep，它不知道原本宣告了什麼，寫下去會把自己的設定蓋到別人開始的嘗試上），而是**在 `begin()` 建立 `pending` 列時就寫入宣告值**，`complete()` 照舊以回應實際回報的值覆寫三者。語意分野寫在 query 註解裡：`completed` 列上這三欄是「實際產出這份判定的是什麼」，`pending`／`failed` 列上是「宣告要用什麼」，`status` 已經讓兩者不會混淆。三個 NULL 各自是陳述而非缺漏（無 judge／平台未宣告 prompt 版本／快照沒凍結 rubric），與 0024 對 `rubric_version` 的既有規則同一條理由。同批修掉一個反向錯誤：**無 acceptance criteria 的路徑原本會記下 judge model**，但該路徑在抵達 judge 之前就回傳——那是替一通從未發生的計費呼叫具名。`contracts/openapi/public.yaml` 的 `judge_model` 說明同步改正（原文寫「即使每條判定都來自規則也會記錄」，與新行為衝突），`task gen:openapi` 重生 TS 與 Go stub。兩個方向皆做破壞驗證：漏寫值會紅，寫假值也會紅。 |

## 6. 執行總結（2026-08-20）

DDD-001～014 全數結案（DDD-006 與 DDD-007 依執行時盤點調整裁定，理由行內記錄）。負責人授權「依最佳實務決策、詳實記錄」下的裁定全部落於 ADR-032／本 ledger／各 commit message。

**Phase 4（DDD-016～026）於同日追加並全數結案**，起因為 Phase 0～3 收斂後的架構審視。**DDD-015（存量漂移清除）亦已結案**：`db/query-owners.yaml` 的 `allow:` 由 15 條降至 **0 條**，經 DDD-019（路徑 2）、DDD-020（路徑 4）、DDD-025（路徑 3）、DDD-026（路徑 1）四批清除。

最後兩批原本被記為「卡在產品與合規決策」，因為 ADR-033 為它們提的補法是事件化。[ADR-034](../adr/ADR-034-cross-context-writes-close-by-inversion-not-by-events.md) 裁定**不採該補法**，改用與前兩批相同的依賴反轉，於是那兩個決策（CORE-007 在最終一致下的「清完了」判準、M1 Explorer 能否接受匯入後的可發現性窗口）**不需要被回答**——它們是事件化才會產生的問題。兩個交易保證原封不動：帳號刪除仍是單一交易的全有全無，匯入完成的當下仍然搜得到。代價是 composition root 變大、依賴從編譯期變執行期，補償為 fail-closed 與 wiring 測試（ADR-034 §影響）。

**Phase 5（DDD-028～030）於 2026-08-21 追加並結案**：`allow:` 清空後，ADR-033／034 各自留下的「read 何時強制」待決策失去了阻擋理由，由 [ADR-035](../adr/ADR-035-read-ownership-enforcement-and-context-map-completeness.md) 回答為「強制，棘輪形狀與 write 相同」，同批讓 ADR-032 §1 對照表第一次被 CI 對帳。read 的存量 35 條凍結為待辦（見下），**不是清完了**——與 write 側不同，這一批只裝籬笆。

執行期間發現、**已列管未修**的殘項：

- ~~`run/job.go` `settle` 依賴轉移表列首為 happy path~~ → **DDD-023 已修**。
- ~~`identity.PurgeExpiredAccounts` 對單一帳號的 purge 失敗只 log 並 `continue`，最後回 `nil`~~ → **2026-08-21 已修**（DDD-027）：失敗仍 log 並 `continue`（一個壞帳號不該卡住整批），但以 `errors.Join` 回傳，`purged` 保持「真的清掉幾筆」的原意而 exit code 誠實。同批修掉 `cmd/maintenance` 一個會被此改動引發的回歸：原本帳號錯誤會提前 return，使同一支命令的 session 清理被跳過——那與被修的毛病同型。兩個 sweep 現在都跑、錯誤 join。原子性測試同步從「記錄 `err=<nil>`」改為斷言錯誤必須浮上來，並經破壞驗證。
- ~~`FailEvaluation` 只寫 `evidence_complete`，ADR-026 決策 1 要求的三個值留 NULL~~ → **DDD-030 已修**（改在 `begin()` 寫宣告值，非在 `fail()` 補寫，理由見該列）。
- `db/migrations/0024` 的 trigger 註解稱 `failed` 列保持可寫是為了「讓 retry 把它變成判定」，但 `CompleteEvaluation` 帶 `status = 'pending'` 述詞，無任何路徑會如此——程式比 DB 嚴，非違規，但註解描述了一個不存在的機制。migration 已套用故不原地改，需要時以新 migration 或文件更正（DDD-022 行內）。
- `eval/reconcile.go` 以裸 SQL 讀 `evaluations`，唯讀且同 context 故非 ADR-033 違規，但繞過宣告，`db/query-owners.yaml` 看不見它（DDD-022／DDD-024 行內）。**read ownership 已於 DDD-028 開始強制，這一條沒有跟著收**：它今天恰好讀自家表故結果不受影響，但換成別人的表，`raw_sql_allow:` 的 tripwire 也不會響（那道只看 DML）。要封死只能把 pool 收在只暴露 sqlc 的 wrapper 後面，成本與拆 sqlc per-context package 同級，維持「等存量清完再評估」（ADR-035 已知盲點）。
- `apiserver`、`eval`、`registry` 三個 package 各自 `DROP SCHEMA public` 並套 migration，共用單一 `SKILLHUB_TEST_DATABASE_URL`。`go test ./...` 並行執行 package 時互相摧毀 schema（CI 有設該環境變數，會紅）。現以各自 `TestMain` 持有 session advisory lock 至整個 package 跑完序列化；**新增第四個重置該資料庫的 package 時必須一併取鎖**，漏取會以 `relation ... does not exist` 大聲失敗而非靜默（DDD-021 行內）。
- `read_allow:` 的 35 條（47 組 (query, caller)、56 個呼叫點）是 DDD-028 導入時凍結的存量，**七群各有清除方向但都還沒做**。**B 組優先**：`LockSkillForRestriction`／`LockTestCase` 是正確性問題不是整潔問題——DDD-020／ADR-034 反轉了寫入半邊，`SELECT … FOR UPDATE` 留在呼叫端，owner 無法保證自己的寫入被序列化，而它看起來已經收過了。逐組判讀見 `db/query-owners.yaml` 的分組註解與 [ADR-035](../adr/ADR-035-read-ownership-enforcement-and-context-map-completeness.md)「存量」節。
- **`trace_events` 的保存期清除沒有實作**。六處文件稱「retention 是 DROP PARTITION」，實際上只有 `trace_events_2026_08` 一個月分割表，`0019` 另建了 `trace_events_default`，**沒有任何程式建立或丟棄分割**。2026-09-01 起所有 trace event 會落進 default 分割，而按月丟分割的做法永遠碰不到它。`analytics_events` 形狀相同，差別在 `purge-analytics` 至少會 DELETE。此項在 DDD-030 調查 `FailEvaluation` 的資料保存前提時發現，屬部署期風險而非 DDD 邊界問題，故未在本計畫內修。
- trace 同批事件 `occurred_at` 相同導致的排序不定 flake（DDD-005 行內；與債務帳 `TRACE-SEQ-001` 同根）。
- outbox poison 隔離是最小版：無自動重放工具、Prometheus rule 屬 `O11Y-PROMTOOL-001`（目錄 §5 行內）。
- 事件目錄缺口 6（aggregate version）依裁定保留 open，第一個需要順序的 consumer 出現才補。
- `m2/README.md` 的「`RunPermissionSummary` 未宣告 `estimated_cost`」註記已證實過時（`02` 已修正，m2 為凍結目錄故不回改）。
- packaging 的 `rfc3339` NULL→epoch 行為若日後判定非刻意，需連同 manifest 版本一起決策（DDD-009 行內）。

## 4. 不做什麼（ponytail 界線）

- 不鋪 repository interface：sqlc per-context queries ＋ package 邊界已是等價封裝。
- 不做 event sourcing、不引 CQRS 框架：`reindex` 投影已是夠用的手工 CQRS。
- 不拆微服務：ADR-010 條件未觸發；本計畫的目的正是讓未來拆分是搬運不是手術。
- 不為 CRUD 稀薄處補 aggregate 儀式：transaction script 在不變量稀薄的 context 是合法模式。
- `internal/platform/` 不改名：`platform/platform` 命名嫌疑在審查中裁定為品味問題，搬動成本大於收益。

## 5. 執行順序與驗證

1. Phase 0 全部 → 一批；DDD-002 的 CI 紅線生效後才開始 Phase 1（先有籬笆再搬東西）。
2. Phase 1 依 DDD-005 → 006 → 008 → 009 順序（005/006 是高衝突區，涉及 `db/queries`／outbox，由主 Agent 序列化；007 是決策項可並行）。
3. Phase 2 在 Phase 1 收斂後啟動；DDD-011 依賴 DDD-003 的 NewApp。Phase 3（DDD-014）在 Phase 0～2 全數收斂後啟動——這是負責人裁定的時點，不提前。
4. 每項完成即更新本 ledger 與 ADR-032 附錄 A（drift 標記移除），遵守「同 commit 改兩處」規則。
5. 驗證一律走既有管道：`task ci`、整合測試（`SKILLHUB_TEST_DATABASE_URL`）、Docker 環境（本機無 Go 工具鏈）。
