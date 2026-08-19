# 技術債修復狀態（2026-08-19）

本檔覆寫 `technical-debt-remediation-2026-08-19.md` 台帳中同名 Finding 的舊狀態。`fixed` 均有自動化測試或 contract drift check；外部決策與部署驗收不會被冒充為程式完成。狀態詞彙固定為：`fixed`＝程式與驗證已完成、`partial`＝已降低風險但仍有明列殘項、`decision`＝等待負責人／法務決策、`deployment`＝只能在目標環境驗證；原始 ledger 的 `open` 只代表首次盤點時尚未處理。

| 狀態 | Finding |
| --- | --- |
| fixed | `LLM-SEC-001`, `GOV-RETENTION-001`, `FE-AUTH-001`, `FE-IMPORT-001`, `FE-CANCEL-001` |
| fixed | `RUN-PERMISSION-001`, `RUN-TESTCASE-001`, `INGEST-SECRET-001`, `IDENTITY-RACE-001`, `HTTP-TIMEOUT-001` |
| fixed | `LLM-EVAL-001`～`LLM-EVAL-006`, `CONTRACT-LLM-422-001`, `LLM-RES-002`, `MODEL-CONFIG-001` |
| fixed | `TEST-DB-DESTRUCTIVE-001`, `TEST-DB-SQL-001`, `SBX-TRANSFER-001`, `SBX-FETCH-LIMIT-001`, `SBX-IMAGE-DIGEST-001` |
| fixed | `FE-EVAL-POLL-001`, `FE-STATE-IDENTITY-001`, `FE-A11Y-FOCUS-001`, `SUPPLY-PREDICATE-001`, `DEV-COMPOSE-BIND-001`, `SBX-DOC-DRIFT-001` |
| fixed | `FE-PAGINATION-001`：Run 與 Test Case 均以 50 筆 page、51st-row probe 提供 Load more。 |
| partial | `LLM-RES-001`：2,000 字元三層上限已完成；分散式 anonymous rate limit 仍屬部署設計。 |
| fixed | `FE-TRACE-SCALE-001`：終態停止輪詢、advanced ingestion cursor／1,000 筆 page、SQL stream health 與 general SQL aggregation 均已完成。 |
| decision/deployment | `DEPLOY-IAC-001`, `RUNTIME-PYTHON-001`, `LLM-EVAL-007`；保留為 release blockers。 |

## 第二輪阻抗審查

- LLM/Evaluation reviewer 找到「單次嘗試會留下永久 pending」；現改成一次執行加一次 recovery-only attempt。Recovery 只把原 revision 標為 failed，不會再次呼叫 Judge。
- Backend reviewer 找到 liveness 被 Bearer 鎖住；`/healthz` 已恢復匿名 200，所有 LLM capability 仍 default-deny。
- Sandbox reviewer 未找到 transfer runtime regression；補上 chunked/無 Content-Length 超限測試與 README 同步。
- generated artifacts 已由 devctl 重生，`gen --check --scope=all` 通過。

## 第三至第五輪補記

- 第三輪找到 completed transaction 遺失 acknowledgment 時仍可能重跑 Judge；River attempt 2 起現為 recovery-only，任何既有 current revision 都不再呼叫 Judge。
- 第四輪找到 recovery 本身若遇資料庫故障仍可能耗盡 job；新增每五分鐘執行、只處理十分鐘以上 pending revision 的 `RecoveryWorker`，只會把同一 revision 收尾為 failed。
- 真實 PostgreSQL/pgvector integration suite 首輪抓到 advisory-lock NUL regression；改用 PostgreSQL 雙 key advisory lock 後，212 個 DB-backed Go tests 全數通過。
- DB-only `immutability_test.sql` 已以一次性 PostgreSQL 容器實跑通過；CI lane 使用同一 pgvector image 內的 `psql`，不依賴 runner 預裝 client。
- source package 在輸出 allow-list 前增加 validation defense-in-depth；credential file 會阻擋，`.git` 等明確不出包的 repository residue 仍安全排除。

## 第六輪阻抗審查

- P0/P1 再次維持零新增。真實 PostgreSQL suite 把純單元測試看不到的問題逼出來：`NextTraceSeq` 參數型別推導、Evaluation current-row 建立競態、Recovery 重複終態化、Outbox 併發 drain，均已修正。
- DB 新增不可繞過的 Run 狀態矩陣與 Trace stream sequence guard；跨 partition 的相同 `(run, attempt, source, seq)` 現會拒絕。`immutability_test.sql` 在真實 pgvector/PostgreSQL 容器通過。
- Evaluation 的 supersede/create 以 per-run advisory transaction lock 序列化；pending revision 只允許一次 `pending -> completed|failed`，兩個 Recovery worker 不會重寫終態或重複產生 completion trace。
- Judge 與 Suggest 的模型用量改為兩筆 append-only ledger，保留 operation、model、prompt version、tokens、nullable gateway cost，並以 Workspace composite FK/predicate scope；LiteLLM metadata 同步帶 `operation=judge|suggest`。
- Python runtime/generated contract parity gate 擴為六組 request/response；它實際抓到並修正 `JudgeRunRequest.final_output` 必填漂移。
- CI supply-chain 補強：GitHub Actions 改 pin commit SHA、Syft/Grype 與 pgvector 改 pin digest、PyYAML pin 版本；runtime image content 變更必須 bump `IMAGE_VERSION`。
- 本輪驗證：platform 492 tests（含 DB）、sandbox 58 tests、web 123 tests、LLM 87 tests 全通過；OpenAPI/sqlc generated check 與 automation check 通過。

## 第七輪阻抗審查

本輪關閉 Trace 規模與 API 邊界：producer `seq` 上限 100,000；missing sequence 回傳有 1,000 筆上限且保留 exact count；`masked_fields` 固定輸出陣列；advanced 以 migration 0034 的 server-owned ingestion cursor 增量讀取；general 由 PostgreSQL 聚合，重複清單有明示的 100 筆上限與 exact total。另補 Test Case 同秒排序 tie-breaker、int32 offset 邊界，以及 release migration runbook 到 schema HEAD 0034。

## 第八輪阻抗審查

三個非 Sol 唯讀審查再發現並關閉：sequence allocation 不等於 commit cursor 的漏事件風險（改為 per-Run transaction serialization 後配置並加交錯交易測試）、General 對惡意 JSON number cast 失敗、終態 Advanced 多頁無法 drain、Evaluator 跨頁非全域順序、Advanced response schema 漂移、前端 Trace DOM 無上限與 Import radio grouping。0032 另補 migration 前既有 logical stream duplicate 的 fail-fast preflight。0034 對既有 partitioned Trace 的強鎖／回填成本已明列為 production-sized clone 演練與維護窗 deployment blocker。

仍需 release/deployment 決策或真實環境證據的項目不偽裝成程式完成：Python 3.12 runtime 定值與 runsc lifecycle、PDM/法務追認、M1 Gate D、0034 production-sized migration 演練，以及 runtime dependency lock 策略。

## 第九、十輪阻抗審查

三個非 Sol 唯讀 Reviewer 針對 Trace/DB、Web/契約與治理重新交叉檢查。第九輪殘留的 lock hierarchy、migration preflight race、General aggregate overflow、Evaluator 全量記憶體與 UI 只能看 tail 均已修正：writer 統一採 run row → ingest → stream 鎖序；聚合在 numeric 階段飽和至 int64；Evaluator 僅保留 500 筆 tail 與各 100 筆 activation/error 並把 truncation 傳給 Judge；Web 改為每頁最多 1,000 筆、前後游標導航且離頁即釋放 cache。

第十輪先發現兩項 P2：API 以可靠的 ingestion cursor 分頁，卻把跨頁描述成全域時間序；終態後 Web 停止 polling，late event 只能靠整頁 reload。契約、工作項與 UI 現已統一為「頁面依接收順序、頁內依事件時間排序」；需要全域 timeline 的 API consumer 必須抓完所有頁後依 `(occurred_at, emitted_by, attempt, seq)` 排序。Web 另提供終態仍可用的手動 Trace refresh，具名測試驗證會重抓目前 cursor。三位最終 Reviewer 分別確認 Trace/DB、治理/契約與 Web 均無仍可重現的 P0–P2。

最終驗證使用全新 `skillhub_round10_test` schema 逐份 transaction 套用 34 份 migration；Platform 495 tests、Web 124 tests＋typecheck＋production build、Sandbox 58 tests＋vet、LLM 87 tests＋ruff、OpenAPI/sqlc generated check 全部通過。Web 的 jsdom `scrollTo`／canvas 訊息仍是測試環境未實作警告，不是失敗。

## 第十一至十四輪：全嚴重度收斂

把複核範圍從 P0～P2 擴為 P0～P3 後，三位非 Sol 唯讀 Reviewer 又找出並關閉四組 Trace 邊界：Sandbox collector 原本永遠重讀 JSONL 首 8 MiB，現改以成功 POST 後才前進的 byte offset；Docker 以對齊 1 MiB 的 bounded `dd` 讀取，避免截斷 multiplex frame。外部 signed ingestion capability 現只接受 `emitted_by=sandbox`，OpenAPI 以 `SandboxTraceEvent` 表達同一限制，並同步 `attempt`／`seq` 最大 100,000。

Collector 依與 HTTP sink 相同的實際 JSON wire encoding 分批，完整但永遠不可能放入 request 的 oversized 單行會被明確捨棄並前進，避免把合法 tail 永久堵住；真實 Docker E2E 使用超過 8 MiB、含大量 `<` 的 payload 驗證 HTML escaping 與尾端事件均不遺失。Evaluator 不再全量 decode Trace：資料庫只選 canonical tail 500 筆與最早 activation／error 各 100 筆，General fold 只物化需要的 scalar；Evaluation event 的 attempt 改從 Workspace-scoped `run_attempts` 最新 persisted row 取得，不再從有界 evidence 猜測。

修正後再次進行全嚴重度阻抗複核；Trace/DB、治理/契約與 Web/contract 三位 Reviewer 均回報 **no actionable findings**。最終驗證：34 份 migration 的新資料庫已套用；Platform 496 tests、Sandbox 61 tests＋vet、Web 124 tests＋typecheck＋production build、LLM 87 tests＋ruff、TypeScript client typecheck、OpenAPI/sqlc generated check 全部通過。
