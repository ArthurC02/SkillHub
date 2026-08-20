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

## 6. 執行總結（2026-08-20）

DDD-001～014 全數結案（DDD-006 與 DDD-007 依執行時盤點調整裁定，理由行內記錄）。負責人授權「依最佳實務決策、詳實記錄」下的裁定全部落於 ADR-032／本 ledger／各 commit message。執行期間發現、**已列管未修**的殘項：

- `run/job.go` `settle` 依賴轉移表列首為 happy path——順序 load-bearing 無測試（DDD-010 行內），宜補 `next(status)` accessor。
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
