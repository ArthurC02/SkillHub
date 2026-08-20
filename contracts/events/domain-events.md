# Platform 領域事件目錄（ADR-008／ADR-032）

- 狀態：目錄首版 2026-08-19；**2026-08-20（DDD-012）§5 七項缺口關閉六項**，值域封閉、同交易型別保證、retention 與 poison 隔離皆已落地。**Go 型別是實作的事實來源**（`apps/platform/internal/platform/db/gen` 的 `OutboxEvent` 信封＋各 producer 的 payload 組裝）；本目錄是**規範與盤點**——列出全部合法 `event_type`、payload 形狀與新增規則。
- 形式：文件目錄，暫無 JSON Schema 與 validator。**第一個非 Go consumer 出現時**，依 [Run Trace 契約](README.md) 的前例補 schema＋validator；在那之前加 schema 是投機成本（目前唯一 consumer 是 process log）。§3 的 `run.*` token 現在**是被機器讀的**：`internal/outbox` 的 conformance test 抓它們，與 Go 常數和 DB `CHECK` 三方比對，所以那些反引號不是排版而是契約。
- 位置理由：`contracts/` 是跨程序介面的唯一來源；領域事件今天雖只在 Go 程序內流動，`internal/run/service.go` 的既有註解早已預告 schema 落點是 `contracts/events/`，本目錄兌現該預告。

## 1. 與其他事件平面的分野

| 平面 | 表 | 本目錄是否涵蓋 |
| --- | --- | --- |
| **領域事件**（Transactional Outbox，ADR-008） | `outbox_events` | ✅ **就是這份** |
| Run Trace（使用者可見時間軸，TRACE-001） | `trace_events` 分割表 | ❌ 見 [README.md](README.md) |
| 產品分析事件（漏斗量測，ADR-029） | `analytics_events` 分割表 | ❌ 封閉四值集合，schema 即白名單，見 ADR-029 |
| Audit（稽核，ADR-009） | `audit_events` | ❌ `run.create`／`run.transition` 等 action 詞彙與本目錄的 event_type **是兩個命名空間**，不得混用 |

## 2. 信封（envelope）

欄位對齊 ADR-008；transport 是 `outbox_events` 表（`db/migrations/0016`，無 immutability trigger——它是傳輸緩衝不是歷史）。

| 欄位 | 語意 | 規約 |
| --- | --- | --- |
| `event_id` | 事件唯一識別 | **consumer 冪等鍵**：at-least-once 送達，以 `event_id` 去重 |
| `event_type` | 目錄 §3 的封閉集合 | 不在目錄內的值不得發出（見 §4 規則 2）；由 `outbox.EventTypes` 與 DB `CHECK` 同時強制 |
| `event_version` | payload 主版本 | 現況一律 `outbox.EventVersion1`；版本規則沿用 Trace 契約 §7（additive 不升主號） |
| `occurred_at` | 交易時間 | 同交易多事件會同值；排序鍵是 `(occurred_at, event_id)`，**跨 aggregate 不保證全序** |
| `correlation_id` | 業務關聯根 | Run 事件＝平台 `run_id`（鐵律 10：不用 Provider 臨時 ID） |
| `causation_id` | 直接成因 | 觸發本事件的 attempt／job／command 識別；填寫規則見下 |
| `workspace_id` | 租戶邊界 | 鐵律 3 |
| `aggregate_type`／`aggregate_id` | 事件所屬 aggregate | 現況 `outbox.AggregateRun`（`"run"`）；詞彙表由本目錄擁有，與 audit 的 resource 詞彙是兩套（§1） |
| `payload` | jsonb | 形狀由 §3 各列定義 |
| `delivery_attempts`／`dead_lettered_at` | 傳輸元資料 | **不是領域資料**：publisher 私有，consumer 不得讀（見 §6） |

**鐵律 9**：事件寫入必須與領域狀態變更同交易。強制方式是型別而非慣例——producer 一律呼叫 `outbox.Insert(ctx, tx pgx.Tx, params)`，直接呼叫 `(*gen.Queries).InsertOutboxEvent` 視同違規。以 pool handle 呼叫在編譯期就過不了。

**`causation_id` 填寫規則**：有 UUID 型別直接成因者一律填，NULL 只允許兩種情形，兩種都由本節列舉、不得擴充：

1. **genesis 事件**（aggregate 的首個事件）——`run.queued`。它之前沒有任何事件，也還沒有 attempt。
2. **成因識別不是 UUID 者**——`run.cleanup_cleaned`／`run.cleanup_failed`。一次 cleanup pass 釋放該 Run 的**全部** attempt，沒有單一 attempt 是它的成因；真正的成因是 `run_cleanup` job，而 River 的 job id 是 bigint。把終態轉移的 attempt id 塞進去既是假資料，也會改變 `CleanupArgs` 的 `ByArgs` 唯一鍵，讓 supervisor 的補派送不再與終態轉移合流，變成兩個 worker 同時拆同一個 sandbox。要真正填上它，需要一個 UUID 型別的 job 識別，那是本目錄之外的變更。

## 3. 事件目錄（現行 11 型，v1＝忠實記錄現況）

### `run` aggregate — 狀態轉移族（producer：Run Orchestration，`internal/run/service.go` `record()`）

| `event_type` | 觸發（同交易的狀態變更） | payload | 備註 |
| --- | --- | --- | --- |
| `run.queued` | `CreateRun`＋Test Case 快照＋`run_execute` 入隊 | `to_status`、`reason:"run requested"` | `from_status` 缺席；`causation_id` NULL（genesis，§2 例外 1） |
| `run.provisioning` `run.preparing` `run.running` `run.evaluating` | `TransitionRun`（非終態） | `to_status`、`from_status`、`reason?`（空值時缺席） | `causation_id`＝attempt ID |
| `run.succeeded` `run.failed` `run.cancelled` `run.timed_out` | `TransitionRun`（終態）＋`run_cleanup` 入隊 | 同上 | 終態另寫 `error` trace 事件（不同平面）；`succeeded`／`failed` 的 consumer 另行入隊 `evaluate_run`（見 §4 規則 5） |

### `run` aggregate — 清理族（producer：`internal/run/cleanup.go` `recordCleanup()`）

| `event_type` | 觸發 | payload | 備註 |
| --- | --- | --- | --- |
| `run.cleanup_cleaned` | `SetRunCleanupStatus` | `cleanup_status` | `causation_id` NULL（成因非 UUID，§2 例外 2） |
| `run.cleanup_failed` | 同上 | `cleanup_status`、`failure_count` | 同上 |

`pending` 與 `cleaning_up` **沒有**對應事件，而且是主動拒絕的：它們不是結果，把「還在清」當成事實發出去只是把猜測寫進事件流。`outbox.CleanupEvent` 對這兩個值回傳 error。

### 概念名對照（ADR-008）

ADR-008 以 PascalCase 過去式描述工作流事件（`RunRequested`、`RunExecutionCompleted`、`CleanupCompleted`…）——那是**概念名**；wire format（`event_type` 字串）以本目錄為準。對照：`RunRequested`≈`run.queued`、`RunExecutionCompleted`≈`run.succeeded|failed|timed_out`、`CleanupCompleted`≈`run.cleanup_cleaned`。ADR-008 的 Skill Ingestion／Packaging／Deletion 工作流與 `EvaluationCompleted` **尚未有任何事件**——新增時依 §4 規則進目錄。

## 4. 規範（新增或修改事件時強制）

1. **命名**：`<aggregate>.<小寫snake過去式事實>`。狀態機鏡像型（`run.<status>`）是既有例外，不再擴散——新事件描述「發生了什麼」，不是「進入了什麼狀態」。
2. **值域封閉**：`event_type` 不得由字串拼接產生；目錄未列的 type 不得發出。**已落地（2026-08-20，DDD-012）**：值域宣告在三處——`outbox.EventTypes`、`db/migrations/0035` 的 `CHECK`、本目錄 §3——`internal/outbox` 的 conformance test 比對三方，任一處漏改即紅。producer 用 `outbox.StatusEvent`／`outbox.CleanupEvent` 映射，未知 status 回 error 讓交易回滾，不會靜默生出新 type。
3. **payload 為 consumer 設計**：欄位存在性必須固定——可缺的欄位明示 nullable，不得「空字串就不放 key」；不得直接重用 audit metadata bag（現況待收斂）。
4. **同 commit 四件事**：新事件＝目錄 §3 加列＋`outbox` 常數與映射＋新 migration 換上新的 `CHECK` 清單＋producer 實作。目錄與程式分岔視同 contract drift，conformance test 就是抓這件事。
5. **觸發源唯一**：跨 context 的「後續反應」以事件 consumer 為唯一觸發源；同 context 的內部工序才可直接入隊 River。2026-08-20（DDD-005）起，`run.succeeded`／`run.failed` 的 consumer（`internal/eval` 的 `RunEventConsumer`）是 `evaluate_run` 入隊的唯一觸發源；終態轉移交易只入隊 `run_cleanup`，那是 Run 自己的內部工序。
6. **consumer 義務**：以 `event_id` 冪等；不得依賴跨 aggregate 順序；對 Run 狀態的認知與 `runs` 表衝突時以表為準（鐵律 5 同理）。

## 5. 缺口盤點

原本七項，**2026-08-20（DDD-012）關閉六項**；第 6 項刻意保持 open。

| # | 缺口 | 狀態 |
| --- | --- | --- |
| 1 | 無 retention | ✅ 已關閉 |
| 2 | 無 poison／DLQ | ✅ 已關閉（最小版，殘餘限制見下） |
| 3 | `causation_id` 只填一半 | ✅ 已關閉（規則修正，非補值） |
| 4 | `aggregate_type` 借用 audit 常數 | ✅ 已關閉 |
| 5 | 同交易無型別保證 | ✅ 已關閉 |
| 6 | 無 aggregate version | ⬜ **open**，刻意 |
| 7 | `event_version` 硬編碼、`event_type` 值域未封閉 | ✅ 已關閉 |

1. **無 retention → 已關閉。** publisher 每輪 pass 先刪 `published_at` 早於 `Worker.PublishedRetention`（預設 7 天）的列，`0035` 補了對應的 partial index。每輪都刪而不是「有成功批次才刪」——閒置系統一樣有上週的列要丟。dead-lettered 的列排除在外（見下一項）。這張表是傳輸緩衝，不是歷史；要留 400 天的是 `audit_events`。
2. **無 poison／DLQ → 已關閉，但是最小版。** `0035` 加 `delivery_attempts`／`dead_lettered_at`；deliver 失敗時在 publisher 的交易之外遞增計數（跟著失敗一起 rollback 的計數永遠到不了門檻），達 `Worker.MaxDeliveryAttempts`（預設 10）即寫入 `dead_lettered_at`、`slog.Error` 並遞增 `skillhub_outbox_dead_lettered_total{event_type}`。被隔離的列從 `ListUnpublishedOutboxEvents` 排除，**head-of-line 阻塞就此解除**。
   **殘餘限制（誠實記錄，不是待辦掩飾）**：(a) 隔離就是隔離——**沒有重放工具、沒有 DLQ 介面**，被隔離的事件由人處理，平台不會自行決定它不重要，所以列**不刪**（retention 也排除）；(b) ADR-008 說的「告警」在本批只到 metric 為止，watch 它的 Prometheus rule 屬 O11Y-PROMTOOL-001，不在本批。
3. **`causation_id` 只填一半 → 已關閉，用規則修正而不是補值。** 盤點結論是「一律填」本來就寫錯了：`run.queued` 是 genesis 事件、沒有先行成因，cleanup 的成因是 River job 而 job id 不是 UUID。硬造一個 attempt id 去填欄位是假資料，而且會連帶破壞 cleanup job 的合流（理由寫在 §2）。**規則改為：有 UUID 型別直接成因者一律填，NULL 只允許 §2 列舉的兩種情形。** 狀態轉移族仍一律填 attempt id，維持原樣。
4. **`aggregate_type` 借用 `audit.ResourceRun` → 已關閉。** 改用 `outbox.AggregateRun`。值仍是 `"run"`，但兩個命名空間不再共用一個常數，任一邊改名不會拖動另一邊。
5. **同交易無型別保證 → 已關閉。** `outbox.Insert(ctx, tx pgx.Tx, params)` 以型別強制交易 handle；兩個 producer 都走它。用 pool handle 寫事件現在是編譯錯誤，不再是慣例。
6. **無 aggregate version → 仍 open，刻意。** ADR-008 說「需要順序時用 Aggregate Version」，欄位仍不存在。目前沒有任何 consumer 依賴 aggregate 內順序（`internal/eval` 對單一事件反應且以 `runs` 表為準），第一個需要順序的 consumer 出現時再補欄位。預先加等於預先猜它需要什麼形狀。
7. **`event_version` 硬編碼、`event_type` 值域未封閉 → 已關閉。** `outbox.EventVersion1` 取代兩處字面量 `1`；值域封閉見 §4 規則 2。

## 6. 傳輸層行為（publisher 私有）

`delivery_attempts`、`dead_lettered_at`、`published_at` 描述的是**這則事件的搬運狀況**，不是領域事實。它們與 §2 的信封欄位分屬兩層，規約如下：

- **consumer 不得讀、不得依賴**。看到 `delivery_attempts = 3` 不代表領域上發生過三次任何事；那只代表某個 consumer 前兩次沒收下。冪等鍵永遠是 `event_id`（§4 規則 6）。
- **一輪 publish pass** = 先跑 retention DELETE → 取一批未發布且未隔離的列（`(occurred_at, event_id)` 排序，上限 200）→ 逐則 deliver → 成功的前綴一次標記 published。失敗即停在該則，其後的列留到下輪。
- **at-least-once 不變**：事件先交付、後標記，中間崩潰會重送。任何時候都不會「標記了但沒送」。
- 同時只有一個 publisher 在跑（`pg_try_advisory_lock`）。這不是效能設計，是避免兩個健康的 worker 對同一批未發布快照各送一次。
