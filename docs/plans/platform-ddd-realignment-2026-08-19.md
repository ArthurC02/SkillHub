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
| DDD-005 | open | `run` → `eval` 事件化 | Run 終態經 Outbox 事件觸發評估入隊（consumer 冪等，ADR-008）；`run` 套件不再 import `eval`；白名單移除該列。前置：債務帳 `OUTBOX-CLAIM-001`（多 publisher claim）宜同批處理。 |
| DDD-006 | open | `ingest` 拆分：套件儲存面歸 Registry context | `SaveVersion`／`PackageFS`／`PackageRoot`／`NewUploadResult` 等套件讀寫抽為 Registry context 的公開 API（新套件或併入 `registry`）；`eval`、`packaging`、`run`、`catalog` 改依該 API，不再 import `ingest`；`ingest` 保留匯入管線（Trust & Supply Chain）；四列 drift 移出白名單。 |
| DDD-007 | open | `testlab` 公開契約面依 UI/UX 導向重整（負責人 2026-08-19 裁定；設計見 [testlab-contract-design-2026-08-19.md](testlab-contract-design-2026-08-19.md)） | 契約面 B：testlab 補 snapshot 讀取面，`run/preflight.go` 改走門面不再直讀 `gen`，`packaging` 繞過 `DecodeCriteria` 的讀法收回；契約面 A：`GET /test-cases?skill_id=`＋list 彙總欄位、`GET /runs?test_case_id=`、Test Case 詳情頁執行歷史區、刪除 Test Case 的使用者面、dataset `expires_at` 上畫面、suggest 改回傳不落庫、preflight 進階限制收合區（hash 涵蓋欄位全數可見）；`public.yaml` 先行（含 `RunPermissionSummary.estimated_cost` 宣告缺漏驗證）；旅程六步各有 UI 測試、新路由入 AuthZ 矩陣。 |
| DDD-008 | open | `catalog` 補 Service 層 | `catalog` 與其他 context 同構（`Handler{Svc}`）；`http.go`（915 行）／`detail.go`（829 行）的查詢與 LLM 呼叫移入 Service；HTTP 層不再直持 pool。 |
| DDD-009 | open | 共用 helper 收斂進 `platform/` | `uuidString`（10 份）、`rfc3339`（3 份）、`envOr`／`addrFromEnv`（4 份）、objstore env 讀取區塊（4 份）收為單一實作；各 context 刪除複本。 |

### Phase 2 — 結構深化（規模成長前的預整）

| ID | 狀態 | 項目 | 完成條件 |
| --- | --- | --- | --- |
| DDD-010 | open | 拆 `internal/run` 內部結構 | 至少分出：狀態機（aggregate，含 transition 表）、應用編排（schedule／dispatch／halt／cleanup）、provider gateway（ACL）三個子包或明確檔案群；`run/service.go`（858 行）不再同時持有三種職責；狀態機可獨立單元測試。 |
| DDD-011 | open | 整合測試搬家＋AuthZ 矩陣補全 | 23 個 `*_integration_test.go` 自 `internal/identity` 移至 `internal/apiserver`（`package apiserver_test`，共用 NewApp）；AuthN/AuthZ 矩陣覆蓋 `router.go` 全部路由（現況約 30/55）；缺席清單見審查：`/test-cases/*`、`/downloads/*`、`/runs/{id}/trace`、`/runs/{id}/evaluation*`、`/admin/dispatch*`、`POST /feedback`、`GET /policy/data-retention`、`GET /runs/{id}/artifacts`。 |
| DDD-012 | open | 事件目錄程式側收斂（落點已裁定並建立：[contracts/events/domain-events.md](../../contracts/events/domain-events.md)，2026-08-19） | 關閉目錄 §5 的七個缺口：outbox retention 刪除、poison／DLQ 與 deliver 失敗可見化（含審查發現的吞錯）、`causation_id` 一律填、`aggregate_type` 常數自 audit 解耦、同交易的型別或 lint 保證、`event_version` 常數化；`event_type` 值域以 conformance test＋DB `CHECK` 封閉（目錄 §4 規則 2）；新增事件遵守「同 commit 三件事」。aggregate version 欄位**不**在本項（第一個需要順序的 consumer 出現才加）。 |
| DDD-013 | open | Ubiquitous language 固定進 AGENTS.md | context ↔ 套件 ↔ 需求 ID 前綴對照表加入 AGENTS.md；「新增 `internal/` 套件必須先登記 context」入開發自動化守則。 |

### Phase 3 — DDD 化完成後（負責人 2026-08-19 裁定時點）

| ID | 狀態 | 項目 | 完成條件 |
| --- | --- | --- | --- |
| DDD-014 | open | 抽離 Policy & Usage context | 前置：Phase 0～2 全數收斂。quota 計數與強制點（現寄居 `run`）、retention 政策（現寄居 `packaging`，見債務帳 `GOV-RETENTION-001`）、用量記錄收攏為獨立套件；`analytics` 維持 Supporting 不併入；ADR-032 context 對照表與 depguard 白名單同批更新；計費接縫（ADR-011 的 Policy & Usage 所有權清單）以此套件為家。 |

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
