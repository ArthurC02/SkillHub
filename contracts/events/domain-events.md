# Platform 領域事件目錄（ADR-008／ADR-032）

- 狀態：目錄首版（2026-08-19）。**Go 型別是實作的事實來源**（`apps/platform/internal/platform/db/gen` 的 `OutboxEvent` 信封＋各 producer 的 payload 組裝）；本目錄是**規範與盤點**——列出全部合法 `event_type`、payload 形狀與新增規則。
- 形式：文件目錄，暫無 JSON Schema 與 validator。**第一個非 Go consumer 出現時**，依 [Run Trace 契約](README.md) 的前例補 schema＋validator；在那之前加 schema 是投機成本（目前唯一 consumer 是 process log）。
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
| `event_type` | 目錄 §3 的封閉集合 | 不在目錄內的值不得發出（見 §4 規則 2） |
| `event_version` | payload 主版本 | 現況一律 `1`；版本規則沿用 Trace 契約 §7（additive 不升主號） |
| `occurred_at` | 交易時間 | 同交易多事件會同值；排序鍵是 `(occurred_at, event_id)`，**跨 aggregate 不保證全序** |
| `correlation_id` | 業務關聯根 | Run 事件＝平台 `run_id`（鐵律 10：不用 Provider 臨時 ID） |
| `causation_id` | 直接成因 | 觸發本事件的 attempt／job／command 識別；見 §5 缺口 3 |
| `workspace_id` | 租戶邊界 | 鐵律 3 |
| `aggregate_type`／`aggregate_id` | 事件所屬 aggregate | 現況 `run`；詞彙表由本目錄擁有（見 §5 缺口 4） |
| `payload` | jsonb | 形狀由 §3 各列定義 |

**鐵律 9**：事件寫入必須與領域狀態變更同交易——`InsertOutboxEvent` 必須以呼叫方的交易 handle 呼叫。此約束目前靠慣例（見 §5 缺口 5）。

## 3. 事件目錄（現行 11 型，v1＝忠實記錄現況）

### `run` aggregate — 狀態轉移族（producer：Run Orchestration，`internal/run/service.go` `record()`）

| `event_type` | 觸發（同交易的狀態變更） | payload | 備註 |
| --- | --- | --- | --- |
| `run.queued` | `CreateRun`＋Test Case 快照＋`run_execute` 入隊 | `to_status`、`reason:"run requested"` | `from_status` 缺席；`causation_id` NULL |
| `run.provisioning` `run.preparing` `run.running` `run.evaluating` | `TransitionRun`（非終態） | `to_status`、`from_status`、`reason?`（空值時缺席） | `causation_id`＝attempt ID |
| `run.succeeded` `run.failed` `run.cancelled` `run.timed_out` | `TransitionRun`（終態）＋`run_cleanup` 入隊 | 同上 | 終態另寫 `error` trace 事件（不同平面）；`succeeded`／`failed` 的 consumer 另行入隊 `evaluate_run`（見 §4 規則 5） |

### `run` aggregate — 清理族（producer：`internal/run/cleanup.go` `recordCleanup()`）

| `event_type` | 觸發 | payload | 備註 |
| --- | --- | --- | --- |
| `run.cleanup_cleaned` | `SetRunCleanupStatus` | `cleanup_status` | `causation_id` NULL |
| `run.cleanup_failed` | 同上 | `cleanup_status`、`failure_count` | 同上 |

### 概念名對照（ADR-008）

ADR-008 以 PascalCase 過去式描述工作流事件（`RunRequested`、`RunExecutionCompleted`、`CleanupCompleted`…）——那是**概念名**；wire format（`event_type` 字串）以本目錄為準。對照：`RunRequested`≈`run.queued`、`RunExecutionCompleted`≈`run.succeeded|failed|timed_out`、`CleanupCompleted`≈`run.cleanup_cleaned`。ADR-008 的 Skill Ingestion／Packaging／Deletion 工作流與 `EvaluationCompleted` **尚未有任何事件**——新增時依 §4 規則進目錄。

## 4. 規範（新增或修改事件時強制）

1. **命名**：`<aggregate>.<小寫snake過去式事實>`。狀態機鏡像型（`run.<status>`）是既有例外，不再擴散——新事件描述「發生了什麼」，不是「進入了什麼狀態」。
2. **值域封閉**：`event_type` 不得由字串拼接產生（現況 `"run."+string(status)` 是待收斂項）；目錄未列的 type 不得發出。落地後由 conformance test＋DB `CHECK` 強制（DDD-012）。
3. **payload 為 consumer 設計**：欄位存在性必須固定——可缺的欄位明示 nullable，不得「空字串就不放 key」；不得直接重用 audit metadata bag（現況待收斂）。
4. **同 commit 三件事**：新事件＝目錄加列＋producer 實作＋（存在 conformance test 後）測試更新。目錄與程式分岔視同 contract drift。
5. **觸發源唯一**：跨 context 的「後續反應」以事件 consumer 為唯一觸發源；同 context 的內部工序才可直接入隊 River。2026-08-20（DDD-005）起，`run.succeeded`／`run.failed` 的 consumer（`internal/eval` 的 `RunEventConsumer`）是 `evaluate_run` 入隊的唯一觸發源；終態轉移交易只入隊 `run_cleanup`，那是 Run 自己的內部工序。
6. **consumer 義務**：以 `event_id` 冪等；不得依賴跨 aggregate 順序；對 Run 狀態的認知與 `runs` 表衝突時以表為準（鐵律 5 同理）。

## 5. 已知缺口（DDD-012 的完成條件即關閉這些）

1. **無 retention**：`0016` 註解假設 publisher 會刪已排空的列，DELETE 從未實作——已發布列永久累積。
2. **無 poison／DLQ**：deliver 失敗現在會 `slog.Error`（含 `event_id`／`event_type`）並讓該批 publish 回錯，由 River 重試（2026-08-20，DDD-005 同批）；但仍無隔離與告警，一個永遠失敗的 consumer 會無限期擋住其後的 backlog（ADR-008 要求有 DLQ）。
3. **`causation_id` 只填一半**：create 與 cleanup 路徑為 NULL。規則：一律填直接成因。
4. **`aggregate_type` 借用 `audit.ResourceRun` 常數**：領域詞彙與 audit 詞彙的隱性耦合，應由本目錄側擁有常數。
5. **同交易無型別保證**：非交易 handle 呼叫 `InsertOutboxEvent` 編譯得過；以 lint 或 helper 型別收緊。
6. **無 aggregate version**：ADR-008 說「需要順序時用 Aggregate Version」，欄位不存在；第一個需要順序的 consumer 出現時補欄位，不預先加。
7. **`event_version` 硬編碼**：兩個呼叫點各寫 `1`；常數化並綁定本目錄版本節。
